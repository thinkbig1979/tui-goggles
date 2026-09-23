// Package terminal provides a virtual terminal emulator that can capture
// the screen state of TUI applications running in a PTY.
package terminal

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// maxTerminalDimension is the maximum allowed terminal size to prevent overflow.
const maxTerminalDimension = math.MaxUint16

// Terminal wraps a PTY and virtual terminal emulator to capture TUI output.
type Terminal struct {
	cmd     *exec.Cmd
	ptyFile *os.File
	writeMu sync.Mutex // serializes input and query replies to the PTY
	resp    *responder
	vt      vt10x.Terminal
	rows    int
	cols    int
	mu      sync.Mutex
	done    chan struct{} // closed when the PTY read loop ends
	err     error

	exited    chan struct{} // closed when the process has exited
	waitErr   error
	grace     time.Duration
	closeOnce sync.Once
	endedBy   string
}

// Options configures the terminal emulator.
type Options struct {
	Rows int
	Cols int
	Env  []string
	// FG and BG are the colors reported for OSC 10 and OSC 11 queries, as
	// "#rrggbb". They default to white on black.
	FG, BG string
	// Grace is how long Close waits for the process to exit after SIGHUP
	// before sending SIGKILL. Zero kills immediately.
	Grace time.Duration
}

// DefaultOptions returns sensible defaults for terminal size.
func DefaultOptions() Options {
	return Options{
		Rows: 24,
		Cols: 80,
	}
}

// New creates a new terminal emulator for the given command.
func New(command string, args []string, opts Options) (*Terminal, error) {
	if opts.Rows == 0 {
		opts.Rows = 24
	}
	if opts.Cols == 0 {
		opts.Cols = 80
	}

	// Validate dimensions to prevent overflow
	if opts.Rows < 0 || opts.Rows > maxTerminalDimension {
		return nil, fmt.Errorf("rows must be between 0 and %d", maxTerminalDimension)
	}
	if opts.Cols < 0 || opts.Cols > maxTerminalDimension {
		return nil, fmt.Errorf("cols must be between 0 and %d", maxTerminalDimension)
	}

	fg, err := ParseColor(defaultString(opts.FG, "#ffffff"))
	if err != nil {
		return nil, fmt.Errorf("foreground: %w", err)
	}
	bg, err := ParseColor(defaultString(opts.BG, "#000000"))
	if err != nil {
		return nil, fmt.Errorf("background: %w", err)
	}

	cmd := exec.Command(command, args...)
	cmd.Env = buildEnv(os.Environ(), opts.Env)

	// Start command with PTY first so we can use it as the vt10x writer
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.Rows), //nolint:gosec // validated above
		Cols: uint16(opts.Cols), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	t := &Terminal{
		cmd:     cmd,
		ptyFile: ptmx,
		rows:    opts.Rows,
		cols:    opts.Cols,
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
		grace:   opts.Grace,
	}
	// Create virtual terminal with PTY as writer for built-in query responses
	// vt10x will automatically respond to DSR (ESC[5n, ESC[6n) queries
	t.vt = vt10x.New(
		vt10x.WithSize(opts.Cols, opts.Rows),
		vt10x.WithWriter(lockedWriter{w: ptmx, mu: &t.writeMu}),
	)
	t.resp = newResponder(ptmx, &t.writeMu, t.Size, fg, bg)

	// Start reading from PTY and feeding to virtual terminal
	go t.readLoop()

	// Reap the process as soon as it exits
	go func() {
		t.waitErr = cmd.Wait()
		close(t.exited)
	}()

	return t, nil
}

// readLoop continuously reads from the PTY and updates the virtual terminal.
// It intercepts terminal queries (DSR, DA1, etc.) and responds appropriately
// so that TUI applications like Bubble Tea can render properly.
func (t *Terminal) readLoop() {
	defer close(t.done)

	reader := bufio.NewReader(t.ptyFile)
	buf := make([]byte, 4096)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			t.write(t.resp.flush())
			if err != io.EOF {
				t.mu.Lock()
				t.err = err
				t.mu.Unlock()
			}
			return
		}

		if n > 0 {
			// Answer terminal queries and filter what vt10x can't parse
			t.write(t.resp.process(buf[:n]))
		}
	}
}

func (t *Terminal) write(data []byte) {
	if len(data) > 0 {
		t.mu.Lock()
		_, _ = t.vt.Write(data)
		t.mu.Unlock()
	}
}

// lockedWriter serializes writes to the PTY.
type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// Variables that describe the real terminal (or multiplexer) tui-goggles
// was started from, or force color behavior. They are not passed to the
// app, so its color profile and feature detection do not depend on where
// tui-goggles runs.
var terminalEnvNames = map[string]bool{
	"TERM": true, "COLORTERM": true, "COLORFGBG": true, "TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true,
	"TERMINAL_EMULATOR": true, "TERM_SESSION_ID": true, "LC_TERMINAL": true, "LC_TERMINAL_VERSION": true,
	"VTE_VERSION": true, "KONSOLE_VERSION": true, "WT_SESSION": true, "WT_PROFILE_ID": true,
	"ITERM_SESSION_ID": true, "ITERM_PROFILE": true, "TMUX": true, "TMUX_PANE": true, "STY": true,
	"NO_COLOR": true, "FORCE_COLOR": true, "CLICOLOR": true, "CLICOLOR_FORCE": true,
	"SSH_TTY": true, "SSH_CONNECTION": true, "SSH_CLIENT": true,
}

var terminalEnvPrefixes = []string{
	"KITTY_", "WEZTERM_", "ALACRITTY_", "GHOSTTY_", "KONSOLE_", "ZELLIJ", "HERDR_", "TILIX_", "TERMINATOR_",
}

// defaultEnv is the terminal identity the app sees unless -env overrides it.
var defaultEnv = []string{"TERM=xterm-256color", "COLORTERM=truecolor"}

// buildEnv returns the app environment: the inherited environment without
// terminal-specific variables, then defaultEnv, then extra. Later entries
// win for duplicate keys, so extra can override anything.
func buildEnv(inherited, extra []string) []string {
	env := make([]string, 0, len(inherited)+len(defaultEnv)+len(extra))
	for _, kv := range inherited {
		name, _, _ := strings.Cut(kv, "=")
		if isTerminalEnv(name) {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, defaultEnv...)
	return append(env, extra...)
}

func isTerminalEnv(name string) bool {
	if terminalEnvNames[name] {
		return true
	}
	for _, p := range terminalEnvPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// PrivateModeSet reports whether the application has set DEC private mode
// m (for example 2004, bracketed paste). Only modes the emulator tracks are
// reported; others are always false.
func (t *Terminal) PrivateModeSet(m int) bool {
	return t.resp.PrivateMode(m)
}

// Screenshot captures the current terminal screen as a text grid.
func (t *Terminal) Screenshot() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.String()
}

// ScreenshotWithCursor captures the screen along with cursor position info.
// Returns: screen content, cursor column (0-indexed), cursor row (0-indexed), cursor visible.
func (t *Terminal) ScreenshotWithCursor() (screen string, cursorCol, cursorRow int, cursorVisible bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	screen = t.vt.String()
	cursor := t.vt.Cursor()
	cursorCol = cursor.X
	cursorRow = cursor.Y
	cursorVisible = t.vt.CursorVisible()

	return screen, cursorCol, cursorRow, cursorVisible
}

// SendKeys sends keystrokes to the running application.
func (t *Terminal) SendKeys(keys string) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err := t.ptyFile.WriteString(keys)
	return err
}

// Wait waits for the command to exit.
func (t *Terminal) Wait() error {
	<-t.exited
	return t.waitErr
}

// WaitForStable waits until the screen content stabilizes (no changes for duration).
func (t *Terminal) WaitForStable(timeout, stableDuration time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastScreen := ""
	stableSince := time.Time{}

	for time.Now().Before(deadline) {
		screen := t.Screenshot()

		if screen != lastScreen {
			lastScreen = screen
			stableSince = time.Now()
		} else if !stableSince.IsZero() && time.Since(stableSince) >= stableDuration {
			return nil
		}

		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for stable screen")
}

// WaitForText waits until the specified text appears on screen.
func (t *Terminal) WaitForText(text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		screen := t.Screenshot()
		if containsText(screen, text) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for text: %q", text)
}

// WaitForTextGone waits until the specified text is no longer on screen.
func (t *Terminal) WaitForTextGone(text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !containsText(t.Screenshot(), text) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for text to disappear: %q", text)
}

// Close ends the command the way closing a real terminal window would: it
// sends SIGHUP to the process group and waits up to the grace period for the
// process to exit (so it can save state), then sends SIGKILL. The PTY stays
// open and is read during the grace period, so the process never blocks on
// output while shutting down. Close is safe to call more than once.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		t.endedBy = t.stop()
		if t.ptyFile != nil {
			_ = t.ptyFile.Close()
		}
		<-t.done
	})
	return nil
}

// stop ends the process and reports how it ended.
func (t *Terminal) stop() string {
	select {
	case <-t.exited:
		return EndedExited
	default:
	}
	if t.cmd.Process == nil {
		return EndedExited
	}
	// The child is a session leader (pty.Start uses Setsid), so -pid
	// addresses its whole process group.
	pgid := -t.cmd.Process.Pid

	if t.grace > 0 {
		_ = syscall.Kill(pgid, syscall.SIGHUP)
		select {
		case <-t.exited:
			return EndedHangup
		case <-time.After(t.grace):
		}
	}
	_ = syscall.Kill(pgid, syscall.SIGKILL)
	<-t.exited
	return EndedKilled
}

// How the process ended, as reported by ExitStatus.
const (
	EndedExited = "exited" // exited on its own before Close
	EndedHangup = "hangup" // exited after SIGHUP, within the grace period
	EndedKilled = "killed" // killed with SIGKILL
)

// ExitStatus describes how the process ended.
type ExitStatus struct {
	// EndedBy is EndedExited, EndedHangup or EndedKilled.
	EndedBy string `json:"ended_by"`
	// ExitCode is the exit code, or -1 if the process was ended by a signal.
	ExitCode int `json:"exit_code"`
	// Signal names the signal that ended the process, if any.
	Signal string `json:"signal,omitempty"`
}

// ExitStatus returns how the process ended. It is only meaningful after Close.
func (t *Terminal) ExitStatus() ExitStatus {
	st := ExitStatus{EndedBy: t.endedBy, ExitCode: -1}
	if t.cmd.ProcessState == nil {
		return st
	}
	st.ExitCode = t.cmd.ProcessState.ExitCode()
	if ws, ok := t.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		st.Signal = signalName(ws.Signal())
	}
	return st
}

func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGSEGV:
		return "SIGSEGV"
	case syscall.SIGABRT:
		return "SIGABRT"
	case syscall.SIGPIPE:
		return "SIGPIPE"
	}
	return fmt.Sprintf("signal %d", int(sig))
}

// Exited reports whether the process has already exited on its own.
func (t *Terminal) Exited() bool {
	select {
	case <-t.exited:
		return true
	default:
		return false
	}
}

// Resize changes the terminal size.
func (t *Terminal) Resize(cols, rows int) error {
	// Validate dimensions to prevent overflow
	if rows < 1 || rows > maxTerminalDimension {
		return fmt.Errorf("rows must be between 1 and %d", maxTerminalDimension)
	}
	if cols < 1 || cols > maxTerminalDimension {
		return fmt.Errorf("cols must be between 1 and %d", maxTerminalDimension)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Resize the emulator first so the redraw the app does on SIGWINCH is
	// parsed at the new size. Resizing in place keeps the screen contents and
	// the PTY writer used for query replies.
	t.vt.Resize(cols, rows)

	err := pty.Setsize(t.ptyFile, &pty.Winsize{
		Rows: uint16(rows), //nolint:gosec // validated above
		Cols: uint16(cols), //nolint:gosec // validated above
	})
	if err != nil {
		t.vt.Resize(t.cols, t.rows)
		return err
	}

	t.rows = rows
	t.cols = cols
	return nil
}

// Size returns the current terminal dimensions.
func (t *Terminal) Size() (cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cols, t.rows
}

// IsRunning returns true if the command is still running.
func (t *Terminal) IsRunning() bool {
	return !t.Exited()
}

// containsText checks if the screen contains the given text.
func containsText(screen, text string) bool {
	return text != "" && screen != "" && strings.Contains(screen, text)
}

// MouseModes reports which mouse reporting modes the application has enabled.
func (t *Terminal) MouseModes() MouseModes {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.vt.Mode()
	return MouseModes{
		Buttons:      m&vt10x.ModeMouseMask != 0,
		ButtonMotion: m&(vt10x.ModeMouseMotion|vt10x.ModeMouseMany) != 0,
		AnyMotion:    m&vt10x.ModeMouseMany != 0,
		SGR:          m&vt10x.ModeMouseSgr != 0,
	}
}

// MouseModes describes the enabled mouse reporting modes.
type MouseModes struct {
	Buttons      bool // any tracking mode (9, 1000, 1002, 1003)
	ButtonMotion bool // motion while a button is held (1002 or 1003)
	AnyMotion    bool // all motion (1003)
	SGR          bool // SGR extended coordinates (1006)
}

// Color is a cell color: 0-255 are palette indexes, DefaultFG and DefaultBG
// are the terminal defaults, and other values are 24-bit RGB (r<<16|g<<8|b).
type Color uint32

// Default colors.
const (
	DefaultFG = Color(vt10x.DefaultFG)
	DefaultBG = Color(vt10x.DefaultBG)
)

// Attr is a set of cell attributes.
type Attr uint16

// Cell attributes (the same bits vt10x uses internally).
const (
	AttrReverse   Attr = 1 << 0
	AttrUnderline Attr = 1 << 1
	AttrBold      Attr = 1 << 2
	AttrItalic    Attr = 1 << 4
	AttrBlink     Attr = 1 << 5
	attrMask           = AttrReverse | AttrUnderline | AttrBold | AttrItalic | AttrBlink
)

// Cell is one screen cell with its style.
//
// FG and BG are the colors as the application set them: for reverse-video
// cells they are swapped back (the emulator stores them swapped) and
// AttrReverse is set. Bold text in colors 0-7 is stored brightened (8-15).
type Cell struct {
	Char   rune
	FG, BG Color
	Attrs  Attr
}

// Cells returns a snapshot of every cell on screen, indexed [row][col].
func (t *Terminal) Cells() [][]Cell {
	t.mu.Lock()
	defer t.mu.Unlock()
	cols, rows := t.vt.Size()
	grid := make([][]Cell, rows)
	for y := 0; y < rows; y++ {
		grid[y] = make([]Cell, cols)
		for x := 0; x < cols; x++ {
			g := t.vt.Cell(x, y)
			c := Cell{Char: g.Char, FG: Color(g.FG), BG: Color(g.BG), Attrs: Attr(g.Mode) & attrMask}
			if c.Char == 0 {
				c.Char = ' '
			}
			if c.Attrs&AttrReverse != 0 {
				c.FG, c.BG = c.BG, c.FG
			}
			grid[y][x] = c
		}
	}
	return grid
}
