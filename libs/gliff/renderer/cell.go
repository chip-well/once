package renderer

import (
	"strconv"
	"strings"
)

// ColorType represents the type of color encoding.
type ColorType uint8

const (
	ColorDefault ColorType = iota // No color / terminal default
	ColorBasic                    // Basic 16 colors (0-15)
	Color256                      // 256-color palette (0-255)
	ColorRGB                      // 24-bit true color
)

// Color represents a foreground or background color.
type Color struct {
	Type  ColorType
	Value uint32 // Basic: 0-15, 256: 0-255, RGB: 0xRRGGBB
}

// DefaultColor returns a color representing the terminal default.
func DefaultColor() Color {
	return Color{Type: ColorDefault}
}

// BasicColor returns a basic 16-color palette color.
func BasicColor(index uint8) Color {
	return Color{Type: ColorBasic, Value: uint32(index & 0x0F)}
}

// PaletteColor returns a 256-color palette color.
func PaletteColor(index uint8) Color {
	return Color{Type: Color256, Value: uint32(index)}
}

// RGBColor returns a 24-bit true color.
func RGBColor(r, g, b uint8) Color {
	return Color{Type: ColorRGB, Value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
}

// Equal returns true if two colors are identical.
func (c Color) Equal(other Color) bool {
	return c == other
}

// Style represents the visual attributes of a cell.
type Style struct {
	FG            Color
	BG            Color
	Bold          bool
	Dim           bool
	Italic        bool
	Underline     bool
	Blink         bool
	Reverse       bool
	Hidden        bool
	Strikethrough bool
}

// DefaultStyle returns a style with all defaults (no attributes).
func DefaultStyle() Style {
	return Style{
		FG: DefaultColor(),
		BG: DefaultColor(),
	}
}

// Equal returns true if two styles are identical.
func (s Style) Equal(other Style) bool {
	return s == other
}

// IsDefault returns true if this style has no attributes set.
func (s Style) IsDefault() bool {
	return s == Style{}
}

// Cell represents a single character cell on the terminal.
type Cell struct {
	Rune  rune  // The character (0 for continuation of wide char)
	Width int   // Display width: 0 for continuation, 1 for normal, 2 for wide
	Style Style // Visual attributes
}

// EmptyCell returns a cell representing an empty space with default style.
func EmptyCell() Cell {
	return Cell{Rune: ' ', Width: 1, Style: DefaultStyle()}
}

// Equal returns true if two cells are identical.
func (c Cell) Equal(other Cell) bool {
	return c == other
}

// sgrSequence returns the SGR escape sequence to transition from one style to another.
func sgrSequence(from, to Style) string {
	if from == to {
		return ""
	}
	if to.IsDefault() {
		return SGRReset
	}

	// Build incremental sequence (only the differences)
	var codes []string

	if from.FG != to.FG {
		codes = append(codes, colorToSGR(to.FG, true))
	}
	if from.BG != to.BG {
		codes = append(codes, colorToSGR(to.BG, false))
	}

	type attrDiff struct {
		from, to bool
		on, off  int
	}
	for _, a := range [...]attrDiff{
		{from.Bold, to.Bold, SGRBold, SGRBoldOff},
		{from.Dim, to.Dim, SGRDim, SGRDimOff},
		{from.Italic, to.Italic, SGRItalic, SGRItalicOff},
		{from.Underline, to.Underline, SGRUnderline, SGRUnderlineOff},
		{from.Blink, to.Blink, SGRBlink, SGRBlinkOff},
		{from.Reverse, to.Reverse, SGRReverse, SGRReverseOff},
		{from.Hidden, to.Hidden, SGRHidden, SGRHiddenOff},
		{from.Strikethrough, to.Strikethrough, SGRStrikethrough, SGRStrikethroughOff},
	} {
		if a.from != a.to {
			if a.to {
				codes = append(codes, strconv.Itoa(a.on))
			} else {
				codes = append(codes, strconv.Itoa(a.off))
			}
		}
	}

	if len(codes) == 0 {
		return ""
	}

	incremental := CSI + strings.Join(codes, ";") + "m"

	// With few changes, the incremental path is always shorter than
	// a full reset+set, so skip building the fresh sequence.
	if len(codes) <= 2 {
		return incremental
	}

	// Build fresh sequence (reset + set all attributes) and pick the shorter one
	freshCodes := styleToSGRCodes(to)
	var fresh strings.Builder
	fresh.WriteString(CSI)
	fresh.WriteString("0")
	for _, code := range freshCodes {
		fresh.WriteByte(';')
		fresh.WriteString(code)
	}
	fresh.WriteByte('m')

	if freshStr := fresh.String(); len(freshStr) < len(incremental) {
		return freshStr
	}
	return incremental
}

// styleToSGRCodes returns the SGR codes needed to set the given style from reset.
func styleToSGRCodes(s Style) []string {
	var codes []string

	for _, a := range [...]struct {
		set  bool
		code int
	}{
		{s.Bold, SGRBold},
		{s.Dim, SGRDim},
		{s.Italic, SGRItalic},
		{s.Underline, SGRUnderline},
		{s.Blink, SGRBlink},
		{s.Reverse, SGRReverse},
		{s.Hidden, SGRHidden},
		{s.Strikethrough, SGRStrikethrough},
	} {
		if a.set {
			codes = append(codes, strconv.Itoa(a.code))
		}
	}

	if s.FG.Type != ColorDefault {
		codes = append(codes, colorToSGR(s.FG, true))
	}
	if s.BG.Type != ColorDefault {
		codes = append(codes, colorToSGR(s.BG, false))
	}

	return codes
}

// colorToSGR returns the SGR code string for a color.
func colorToSGR(c Color, foreground bool) string {
	switch c.Type {
	case ColorDefault:
		if foreground {
			return strconv.Itoa(SGRFGDefault)
		}
		return strconv.Itoa(SGRBGDefault)
	case ColorBasic:
		if foreground {
			if c.Value < 8 {
				return strconv.Itoa(SGRFGBlack + int(c.Value))
			}
			return strconv.Itoa(SGRFGBrightBlack + int(c.Value-8))
		}
		if c.Value < 8 {
			return strconv.Itoa(SGRBGBlack + int(c.Value))
		}
		return strconv.Itoa(SGRBGBrightBlack + int(c.Value-8))
	case Color256:
		if foreground {
			return "38;5;" + strconv.Itoa(int(c.Value))
		}
		return "48;5;" + strconv.Itoa(int(c.Value))
	case ColorRGB:
		r := (c.Value >> 16) & 0xFF
		g := (c.Value >> 8) & 0xFF
		b := c.Value & 0xFF
		if foreground {
			return "38;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b))
		}
		return "48;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b))
	}
	return ""
}
