package tui

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers

type recordingScreen struct {
	renders []string
	resizes []WindowSizeMsg
}

func (s *recordingScreen) Render(content string) int {
	s.renders = append(s.renders, content)
	return 0
}

func (s *recordingScreen) Resize(width, height int) {
	s.resizes = append(s.resizes, WindowSizeMsg{Width: width, Height: height})
}

type recordingComponent struct {
	messages []Msg
	initFn   func() Cmd
	updateFn func(Msg) Cmd
	renderFn func() string
}

func (c *recordingComponent) Init() Cmd {
	if c.initFn != nil {
		return c.initFn()
	}
	return nil
}

func (c *recordingComponent) Update(msg Msg) Cmd {
	c.messages = append(c.messages, msg)
	if c.updateFn != nil {
		return c.updateFn(msg)
	}
	return nil
}

func (c *recordingComponent) Render() string {
	if c.renderFn != nil {
		return c.renderFn()
	}
	return "rendered"
}

// runCmd tests

func TestRunCmd(t *testing.T) {
	app := NewApplication(&recordingComponent{})

	// run executes a Cmd via runCmd and returns whatever message it produced,
	// or nil if no message was sent. For non-nil commands, the cmd is wrapped
	// so we can wait for it to complete without any timeouts.
	run := func(cmd Cmd) Msg {
		ch := make(chan Msg, 1)
		if cmd == nil {
			app.runCmd(nil, ch)
			return nil
		}

		var result Msg
		done := make(chan struct{})
		app.runCmd(func() Msg {
			result = cmd()
			close(done)
			return result
		}, ch)
		<-done
		return result
	}

	t.Run("NilCommand", func(t *testing.T) {
		assert.Nil(t, run(nil))
	})

	t.Run("CommandReturnsMessage", func(t *testing.T) {
		assert.Equal(t, KeyMsg{Key{Type: KeyEnter}}, run(func() Msg {
			return KeyMsg{Key{Type: KeyEnter}}
		}))
	})

	t.Run("CommandReturnsNil", func(t *testing.T) {
		assert.Nil(t, run(func() Msg { return nil }))
	})
}

// initializeComponent tests

func TestInitializeComponent(t *testing.T) {
	init := func(comp *recordingComponent) *recordingScreen {
		app := NewApplication(comp)
		scr := &recordingScreen{}
		msgs := make(chan Msg, 10)
		app.initializeComponent(scr, msgs, 80, 24)
		return scr
	}

	t.Run("SendsWindowSize", func(t *testing.T) {
		comp := &recordingComponent{}
		scr := init(comp)

		require.Len(t, comp.messages, 1)
		assert.Equal(t, WindowSizeMsg{Width: 80, Height: 24}, comp.messages[0])
		require.Len(t, scr.renders, 1)
		assert.Equal(t, "rendered", scr.renders[0])
	})

	t.Run("RunsInitCommand", func(t *testing.T) {
		initMsg := KeyMsg{Key{Type: KeyEnter}}
		var result Msg
		done := make(chan struct{})

		comp := &recordingComponent{
			initFn: func() Cmd {
				return func() Msg {
					result = initMsg
					close(done)
					return initMsg
				}
			},
		}
		init(comp)
		<-done

		assert.Equal(t, initMsg, result)
	})
}

// eventLoop tests

func TestEventLoop(t *testing.T) {
	// run pre-loads messages into a buffered channel and runs eventLoop to
	// completion, returning the screen for further assertions.
	run := func(comp *recordingComponent, msgs ...Msg) *recordingScreen {
		app := NewApplication(comp)
		scr := &recordingScreen{}
		ch := make(chan Msg, 10)
		for _, m := range msgs {
			ch <- m
		}
		err := app.eventLoop(scr, ch)
		require.NoError(t, err)
		return scr
	}

	t.Run("QuitMsg", func(t *testing.T) {
		comp := &recordingComponent{}
		scr := run(comp, QuitMsg{})

		assert.Empty(t, comp.messages)
		assert.Empty(t, scr.renders)
	})

	t.Run("WindowSizeMsg", func(t *testing.T) {
		comp := &recordingComponent{}
		scr := run(comp, WindowSizeMsg{Width: 100, Height: 50}, QuitMsg{})

		require.Len(t, scr.resizes, 1)
		assert.Equal(t, WindowSizeMsg{Width: 100, Height: 50}, scr.resizes[0])
		require.Len(t, comp.messages, 1)
		assert.Equal(t, WindowSizeMsg{Width: 100, Height: 50}, comp.messages[0])
		require.Len(t, scr.renders, 1)
	})

	t.Run("RegularMsg", func(t *testing.T) {
		comp := &recordingComponent{}
		scr := run(comp, KeyMsg{Key{Type: KeyEnter}}, QuitMsg{})

		require.Len(t, comp.messages, 1)
		assert.Equal(t, KeyMsg{Key{Type: KeyEnter}}, comp.messages[0])
		require.Len(t, scr.renders, 1)
	})

	t.Run("BatchMsg", func(t *testing.T) {
		comp := &recordingComponent{
			updateFn: func(msg Msg) Cmd {
				return func() Msg { return QuitMsg{} }
			},
		}
		scr := run(comp, BatchMsg{
			func() Msg { return KeyMsg{Key{Type: KeyEnter}} },
		})

		// BatchMsg itself should NOT trigger Update — only the KeyMsg from the batch command
		require.Len(t, comp.messages, 1)
		assert.Equal(t, KeyMsg{Key{Type: KeyEnter}}, comp.messages[0])
		require.Len(t, scr.renders, 1)
	})

	t.Run("Batch", func(t *testing.T) {
		seen := 0
		comp := &recordingComponent{
			updateFn: func(msg Msg) Cmd {
				if km, ok := msg.(KeyMsg); ok && km.Type == KeyEnter {
					return Batch(
						func() Msg { return KeyMsg{Key{Type: KeyTab}} },
						func() Msg { return KeyMsg{Key{Type: KeyEscape}} },
					)
				}
				seen++
				if seen == 2 {
					return func() Msg { return QuitMsg{} }
				}
				return nil
			},
		}
		run(comp, KeyMsg{Key{Type: KeyEnter}})

		// KeyEnter triggers Batch, both batch commands should be processed
		require.Len(t, comp.messages, 3)
		assert.Equal(t, KeyMsg{Key{Type: KeyEnter}}, comp.messages[0])
		// Batch commands run concurrently so order may vary
		assert.ElementsMatch(t,
			[]Msg{KeyMsg{Key{Type: KeyTab}}, KeyMsg{Key{Type: KeyEscape}}},
			comp.messages[1:],
		)
	})

	t.Run("CommandChaining", func(t *testing.T) {
		step := 0
		comp := &recordingComponent{
			updateFn: func(msg Msg) Cmd {
				if _, ok := msg.(KeyMsg); ok {
					step++
					if step == 1 {
						return func() Msg { return KeyMsg{Key{Type: KeyTab}} }
					}
					return func() Msg { return QuitMsg{} }
				}
				return nil
			},
		}
		scr := run(comp, KeyMsg{Key{Type: KeyEnter}})

		require.Len(t, comp.messages, 2)
		assert.Equal(t, KeyMsg{Key{Type: KeyEnter}}, comp.messages[0])
		assert.Equal(t, KeyMsg{Key{Type: KeyTab}}, comp.messages[1])
		assert.Len(t, scr.renders, 2)
	})

	t.Run("ChannelClose", func(t *testing.T) {
		comp := &recordingComponent{}
		app := NewApplication(comp)
		msgs := make(chan Msg, 10)
		close(msgs)

		err := app.eventLoop(&recordingScreen{}, msgs)

		assert.NoError(t, err)
		assert.Empty(t, comp.messages)
	})
}

// mockTerminal implements the terminal interface for testing.
type mockTerminal struct {
	width, height int
	resizeCh      chan struct{}
	input         io.Reader
	output        bytes.Buffer
	enterCalled   bool
	exitCalled    bool
}

func (m *mockTerminal) Write(p []byte) (int, error) { return m.output.Write(p) }
func (m *mockTerminal) EnterFullScreen() error      { m.enterCalled = true; return nil }
func (m *mockTerminal) ExitFullScreen() error       { m.exitCalled = true; return nil }
func (m *mockTerminal) Size() (int, int)            { return m.width, m.height }
func (m *mockTerminal) StartResizeListener()        {}
func (m *mockTerminal) StopResizeListener()         { close(m.resizeCh) }
func (m *mockTerminal) Resized() <-chan struct{}    { return m.resizeCh }
func (m *mockTerminal) Input() io.Reader            { return m.input }

// handleResizeEvents tests

func TestHandleResizeEvents(t *testing.T) {
	mock := &mockTerminal{
		width:    100,
		height:   50,
		resizeCh: make(chan struct{}, 1),
	}

	app := &Application{
		component:    &recordingComponent{},
		mouseTracker: &MouseTracker{},
		term:         mock,
	}
	msgs := make(chan Msg, 10)

	stop := app.handleResizeEvents(msgs)

	mock.resizeCh <- struct{}{}

	select {
	case msg := <-msgs:
		wsm, ok := msg.(WindowSizeMsg)
		require.True(t, ok)
		assert.Equal(t, 100, wsm.Width)
		assert.Equal(t, 50, wsm.Height)
	case <-time.After(time.Second):
		require.Fail(t, "expected WindowSizeMsg")
	}

	stop()
}

// handleInputEvents tests

func TestHandleInputEvents(t *testing.T) {
	// send writes raw bytes into a pipe, feeds them through handleInputEvents,
	// and returns the first message that arrives on the msgs channel.
	send := func(t *testing.T, input []byte) Msg {
		t.Helper()
		pr, pw, err := os.Pipe()
		require.NoError(t, err)
		defer pr.Close()

		mock := &mockTerminal{
			resizeCh: make(chan struct{}),
			input:    pr,
		}
		app := &Application{
			component:    &recordingComponent{},
			mouseTracker: &MouseTracker{},
			term:         mock,
		}
		msgs := make(chan Msg, 10)
		stop := app.handleInputEvents(msgs)
		defer stop()

		pw.Write(input)
		pw.Close()

		select {
		case msg := <-msgs:
			return msg
		case <-time.After(time.Second):
			require.Fail(t, "expected message from input")
			return nil
		}
	}

	t.Run("KeyMsg", func(t *testing.T) {
		msg := send(t, []byte("a"))
		km, ok := msg.(KeyMsg)
		require.True(t, ok)
		assert.Equal(t, KeyRune, km.Type)
		assert.Equal(t, 'a', km.Rune)
	})

	t.Run("MouseMsg", func(t *testing.T) {
		// SGR mouse press: ESC [ < 0 ; 5 ; 10 M (left click at 5,10)
		msg := send(t, []byte("\x1b[<0;5;10M"))
		mm, ok := msg.(MouseMsg)
		require.True(t, ok)
		assert.Equal(t, MouseLeft, mm.Button)
		assert.Equal(t, MousePress, mm.Type)
		assert.Equal(t, 4, mm.X) // 0-indexed
		assert.Equal(t, 9, mm.Y)
	})
}

// Run tests

func TestRun(t *testing.T) {
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	defer pr.Close()

	mock := &mockTerminal{
		width:    80,
		height:   24,
		resizeCh: make(chan struct{}),
		input:    pr,
	}

	comp := &recordingComponent{
		updateFn: func(msg Msg) Cmd {
			if _, ok := msg.(KeyMsg); ok {
				return func() Msg { return QuitMsg{} }
			}
			return nil
		},
	}

	app := NewApplication(comp)
	app.term = mock

	go func() {
		time.Sleep(50 * time.Millisecond)
		pw.Write([]byte("x"))
		pw.Close()
	}()

	err = app.Run()

	assert.NoError(t, err)
	assert.True(t, mock.enterCalled)
	assert.True(t, mock.exitCalled)

	// Component should have received WindowSizeMsg (from initializeComponent) and KeyMsg
	require.Len(t, comp.messages, 2)
	assert.Equal(t, WindowSizeMsg{Width: 80, Height: 24}, comp.messages[0])
	km, ok := comp.messages[1].(KeyMsg)
	require.True(t, ok)
	assert.Equal(t, KeyRune, km.Type)
	assert.Equal(t, 'x', km.Rune)
}
