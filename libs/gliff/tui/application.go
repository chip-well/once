package tui

import (
	"io"

	"github.com/basecamp/gliff/renderer"
)

type screen interface {
	Render(content string) int
	Resize(width, height int)
}

type terminal interface {
	io.Writer
	EnterFullScreen() error
	ExitFullScreen() error
	Size() (int, int)
	StartResizeListener()
	StopResizeListener()
	Resized() <-chan struct{}
	Input() io.Reader
}

type Application struct {
	component    Component
	mouseTracker *MouseTracker
	term         terminal
}

func NewApplication(c Component) *Application {
	return &Application{
		component:    c,
		mouseTracker: defaultMouseTracker,
	}
}

func (a *Application) Run() error {
	closeTerminal, err := a.initializeTerminal()
	if err != nil {
		return err
	}
	defer closeTerminal()

	width, height := a.term.Size()
	screen := renderer.NewScreen(a.term, width, height)
	msgs := make(chan Msg)

	defer a.handleResizeEvents(msgs)()
	defer a.handleInputEvents(msgs)()

	a.initializeComponent(screen, msgs, width, height)
	return a.eventLoop(screen, msgs)
}

// Private

func (a *Application) initializeTerminal() (func() error, error) {
	if a.term == nil {
		t, err := renderer.NewTerminal()
		if err != nil {
			return nil, err
		}
		a.term = t
	}

	if err := a.term.EnterFullScreen(); err != nil {
		return nil, err
	}
	return a.term.ExitFullScreen, nil
}

func (a *Application) handleResizeEvents(msgs chan<- Msg) (stop func()) {
	a.term.StartResizeListener()
	go func() {
		for range a.term.Resized() {
			w, h := a.term.Size()
			msgs <- WindowSizeMsg{Width: w, Height: h}
		}
	}()
	return a.term.StopResizeListener
}

func (a *Application) handleInputEvents(msgs chan<- Msg) (stop func()) {
	input := newInputReader(a.term.Input())
	go func() {
		for key := range input.Keys() {
			msgs <- KeyMsg{key}
		}
	}()
	go func() {
		for mouse := range input.Mouse() {
			msgs <- mouse
		}
	}()
	return input.Stop
}

func (a *Application) initializeComponent(s screen, msgs chan Msg, width, height int) {
	a.runCmd(a.component.Init(), msgs)
	a.runCmd(a.component.Update(WindowSizeMsg{Width: width, Height: height}), msgs)
	s.Render(a.mouseTracker.Sweep(a.component.Render()))
}

func (a *Application) eventLoop(s screen, msgs chan Msg) error {
	for msg := range msgs {
		switch m := msg.(type) {
		case QuitMsg:
			return nil
		case WindowSizeMsg:
			s.Resize(m.Width, m.Height)
		case BatchMsg:
			for _, cmd := range m {
				a.runCmd(cmd, msgs)
			}
			continue
		case MouseMsg:
			m.Target = a.mouseTracker.Resolve(m.X, m.Y)
			msg = m
		}

		cmd := a.component.Update(msg)
		s.Render(a.mouseTracker.Sweep(a.component.Render()))
		a.runCmd(cmd, msgs)
	}
	return nil
}

func (a *Application) runCmd(cmd Cmd, msgs chan<- Msg) {
	if cmd == nil {
		return
	}
	go func() {
		if msg := cmd(); msg != nil {
			msgs <- msg
		}
	}()
}
