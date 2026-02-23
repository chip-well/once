package renderer

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/basecamp/gliff/ansi"
)

// parseContent parses a string into a 2D cell buffer.
// The string may contain ANSI escape sequences for styling.
// Lines are separated by \n. The output is sized to width x height.
func parseContent(content string, width, height int) [][]Cell {
	p := &parser{
		width:  width,
		height: height,
		style:  DefaultStyle(),
	}
	p.initCells()
	p.parse(content)
	return p.cells
}

// parseContentInto parses a string into an existing cell buffer.
// The buffer must already be allocated and cleared.
func parseContentInto(content string, cells [][]Cell, width, height int) {
	p := &parser{
		width:  width,
		height: height,
		cells:  cells,
		style:  DefaultStyle(),
	}
	p.parse(content)
}

// parser converts a string with ANSI escape sequences into a 2D cell grid.
type parser struct {
	width  int
	height int
	cells  [][]Cell
	style  Style

	// Current position
	row, col int
}

// initCells initializes the cell buffer with empty cells.
func (p *parser) initCells() {
	p.cells = make([][]Cell, p.height)
	for i := range p.cells {
		p.cells[i] = make([]Cell, p.width)
		for j := range p.cells[i] {
			p.cells[i][j] = EmptyCell()
		}
	}
}

// parse processes the content using the ANSI lexer.
func (p *parser) parse(content string) {
	lexer := ansi.NewLexer(content)

	for {
		tok := lexer.Next()
		if tok.Type == ansi.EOFToken {
			break
		}

		switch tok.Type {
		case ansi.TextToken:
			p.handleText(tok.Text)
		case ansi.CSIToken:
			p.handleCSI(tok)
		case ansi.ESCToken:
			// Non-CSI escape sequences are ignored
		}
	}
}

// handleText processes a text token.
func (p *parser) handleText(text string) {
	for _, r := range text {
		switch r {
		case '\n':
			p.row++
			p.col = 0
		case '\r':
			p.col = 0
		case '\t':
			// Tab to next 8-column boundary
			nextTab := ((p.col / 8) + 1) * 8
			for p.col < nextTab && p.col < p.width {
				p.setCell(' ', 1)
			}
		default:
			if r >= 32 { // Printable character
				w := runewidth.RuneWidth(r)
				p.setCell(r, w)
			}
			// Control characters (except those handled above) are ignored
		}
	}
}

// handleCSI processes a CSI token.
func (p *parser) handleCSI(tok ansi.Token) {
	params, final := ansi.ParseCSI(tok)
	if final == 'm' {
		// SGR (Select Graphic Rendition)
		p.handleSGR(ansi.ParseSGRParams(params))
	}
	// Other CSI sequences are ignored (cursor movement, etc. in input doesn't make sense)
}

// handleSGR processes SGR (Select Graphic Rendition) parameters.
func (p *parser) handleSGR(params []int) {
	if len(params) == 0 {
		params = []int{0} // Default to reset
	}

	i := 0
	for i < len(params) {
		param := params[i]
		i++

		switch param {
		case 0:
			p.style = DefaultStyle()
		case 1:
			p.style.Bold = true
		case 2:
			p.style.Dim = true
		case 3:
			p.style.Italic = true
		case 4:
			p.style.Underline = true
		case 5:
			p.style.Blink = true
		case 7:
			p.style.Reverse = true
		case 8:
			p.style.Hidden = true
		case 9:
			p.style.Strikethrough = true
		case 22:
			p.style.Bold = false
			p.style.Dim = false
		case 23:
			p.style.Italic = false
		case 24:
			p.style.Underline = false
		case 25:
			p.style.Blink = false
		case 27:
			p.style.Reverse = false
		case 28:
			p.style.Hidden = false
		case 29:
			p.style.Strikethrough = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			p.style.FG = BasicColor(uint8(param - 30))
		case 38:
			color, consumed := parseExtendedColor(params, i)
			p.style.FG = color
			i += consumed
		case 39:
			p.style.FG = DefaultColor()
		case 40, 41, 42, 43, 44, 45, 46, 47:
			p.style.BG = BasicColor(uint8(param - 40))
		case 48:
			color, consumed := parseExtendedColor(params, i)
			p.style.BG = color
			i += consumed
		case 49:
			p.style.BG = DefaultColor()
		case 90, 91, 92, 93, 94, 95, 96, 97:
			p.style.FG = BasicColor(uint8(param - 90 + 8))
		case 100, 101, 102, 103, 104, 105, 106, 107:
			p.style.BG = BasicColor(uint8(param - 100 + 8))
		}
	}
}

// setCell writes a character to the current position with the current style.
func (p *parser) setCell(r rune, width int) {
	if p.row >= p.height {
		return // Off-screen
	}

	// Handle characters based on width
	switch width {
	case 2:
		// Need space for both cells
		if p.col+1 >= p.width {
			// Wide char doesn't fit; fill with space and move to next line
			if p.col < p.width {
				p.cells[p.row][p.col] = Cell{Rune: ' ', Width: 1, Style: p.style}
			}
			p.row++
			p.col = 0
			if p.row >= p.height {
				return
			}
		}
		// First cell holds the character
		p.cells[p.row][p.col] = Cell{Rune: r, Width: 2, Style: p.style}
		p.col++
		// Second cell is a continuation marker
		if p.col < p.width {
			p.cells[p.row][p.col] = Cell{Rune: 0, Width: 0, Style: p.style}
			p.col++
		}
	case 1:
		if p.col < p.width {
			p.cells[p.row][p.col] = Cell{Rune: r, Width: 1, Style: p.style}
			p.col++
		}
	}
	// Zero-width characters (combining marks) are currently ignored
	// A more complete implementation would attach them to the previous character
}

// stripANSI removes ANSI escape sequences from a string.
// Useful for calculating the display width of styled text.
func stripANSI(s string) string {
	var result strings.Builder
	lexer := ansi.NewLexer(s)

	for {
		tok := lexer.Next()
		if tok.Type == ansi.EOFToken {
			break
		}
		if tok.Type == ansi.TextToken {
			result.WriteString(tok.Text)
		}
		// CSI and ESC tokens are skipped
	}

	return result.String()
}

// parseExtendedColor parses an extended color (256-color or RGB) from SGR parameters
// starting at index i. Returns the parsed color and the number of parameters consumed.
func parseExtendedColor(params []int, i int) (Color, int) {
	if i >= len(params) {
		return DefaultColor(), 0
	}
	switch params[i] {
	case 5: // 256 color
		if i+1 < len(params) {
			return PaletteColor(uint8(params[i+1])), 2
		}
		return DefaultColor(), 1
	case 2: // RGB
		if i+3 < len(params) {
			return RGBColor(uint8(params[i+1]), uint8(params[i+2]), uint8(params[i+3])), 4
		}
		return DefaultColor(), 1
	default:
		return DefaultColor(), 1
	}
}
