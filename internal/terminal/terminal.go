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

	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), opts.Env...)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	// Start command with PTY first so we can use it as the vt10x writer
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.Rows), //nolint:gosec // validated above
		Cols: uint16(opts.Cols), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	// Create virtual terminal with PTY as writer for built-in query responses
	// vt10x will automatically respond to DSR (ESC[5n, ESC[6n) queries
	vt := vt10x.New(
		vt10x.WithSize(opts.Cols, opts.Rows),
		vt10x.WithWriter(ptmx),
	)

	t := &Terminal{
		cmd:     cmd,
		ptyFile: ptmx,
		vt:      vt,
		rows:    opts.Rows,
		cols:    opts.Cols,
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
		grace:   opts.Grace,
	}

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
			if err != io.EOF {
				t.mu.Lock()
				t.err = err
				t.mu.Unlock()
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Scan for and respond to terminal queries before passing to vt10x
			data = t.handleTerminalQueries(data)

			if len(data) > 0 {
				t.mu.Lock()
				_, _ = t.vt.Write(data)
				t.mu.Unlock()
			}
		}
	}
}

// handleTerminalQueries scans the output for terminal queries and responds to them.
// It returns the data with query sequences removed (they shouldn't be rendered).
func (t *Terminal) handleTerminalQueries(data []byte) []byte {
	result := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == 0x1b && i+1 < len(data) {
			if skip := t.tryHandleQuery(data, i); skip > 0 {
				i += skip
				continue
			}
		}
		result = append(result, data[i])
		i++
	}

	return result
}

// tryHandleQuery checks if there's a terminal query at position i and handles it.
// Returns the number of bytes to skip if a query was handled, 0 otherwise.
func (t *Terminal) tryHandleQuery(data []byte, i int) int {
	// CSI sequences: ESC [
	if skip := t.tryHandleCSIQuery(data, i); skip > 0 {
		return skip
	}
	// OSC sequences: ESC ]
	if skip := t.tryHandleOSCQuery(data, i); skip > 0 {
		return skip
	}
	return 0
}

// tryHandleCSIQuery handles CSI (Control Sequence Introducer) queries.
func (t *Terminal) tryHandleCSIQuery(data []byte, i int) int {
	if i+2 >= len(data) || data[i+1] != '[' {
		return 0
	}

	// Note: DSR (ESC[5n, ESC[6n) is now handled by vt10x via WithWriter

	// DA1 (Primary Device Attributes): ESC [ c
	if data[i+2] == 'c' {
		t.respondToDA1()
		return 3
	}

	// DA1 alternate form: ESC [ 0 c
	if i+3 < len(data) && data[i+2] == '0' && data[i+3] == 'c' {
		t.respondToDA1()
		return 4
	}

	// DA2 (Secondary Device Attributes): ESC [ > c or ESC [ > 0 c
	if data[i+2] == '>' {
		if i+3 < len(data) && data[i+3] == 'c' {
			t.respondToDA2()
			return 4
		}
		if i+4 < len(data) && data[i+3] == '0' && data[i+4] == 'c' {
			t.respondToDA2()
			return 5
		}
	}

	// XTWINOPS - terminal size queries: ESC [ 1 4 t, ESC [ 1 8 t, ESC [ 1 9 t
	if skip := t.tryHandleXTWINOPS(data, i); skip > 0 {
		return skip
	}

	return 0
}

// tryHandleXTWINOPS handles xterm window operations (size queries).
func (t *Terminal) tryHandleXTWINOPS(data []byte, i int) int {
	// Need at least ESC [ N N t
	if i+4 >= len(data) {
		return 0
	}

	// Check for ESC [ 1 ...
	if data[i+2] != '1' {
		return 0
	}

	// ESC [ 1 4 t - report window size in pixels (we fake it)
	if data[i+3] == '4' && data[i+4] == 't' {
		t.respondToWindowSizePixels()
		return 5
	}

	// ESC [ 1 8 t - report text area size in chars
	if data[i+3] == '8' && data[i+4] == 't' {
		t.respondToTextAreaSize()
		return 5
	}

	// ESC [ 1 9 t - report screen size in chars
	if data[i+3] == '9' && data[i+4] == 't' {
		t.respondToScreenSize()
		return 5
	}

	return 0
}

// tryHandleOSCQuery handles OSC (Operating System Command) queries.
func (t *Terminal) tryHandleOSCQuery(data []byte, i int) int {
	if i+4 >= len(data) || data[i+1] != ']' {
		return 0
	}

	// Background color query: ESC ] 11 ; ...
	if data[i+2] == '1' && data[i+3] == '1' && data[i+4] == ';' {
		if end := t.findOSCEnd(data, i+5); end > i {
			t.respondToBackgroundColorQuery()
			return end - i
		}
	}

	// Foreground color query: ESC ] 10 ; ...
	if data[i+2] == '1' && data[i+3] == '0' && data[i+4] == ';' {
		if end := t.findOSCEnd(data, i+5); end > i {
			t.respondToForegroundColorQuery()
			return end - i
		}
	}

	return 0
}

// findOSCEnd finds the end of an OSC sequence starting from offset.
// Returns the position after the terminator, or -1 if not found.
func (t *Terminal) findOSCEnd(data []byte, offset int) int {
	for i := offset; i < len(data); i++ {
		// BEL (0x07) terminates OSC
		if data[i] == 0x07 {
			return i + 1
		}
		// ST (ESC \) terminates OSC
		if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
			return i + 2
		}
	}
	return -1
}

// respondToDA1 sends primary device attributes response.
// This tells the application we're a VT220-compatible terminal.
// Response: ESC [ ? 6 2 ; 4 c (VT220 with sixel - even though we don't render it)
func (t *Terminal) respondToDA1() {
	// VT220 response with common capabilities
	// 62 = VT220, 4 = sixel (claim support for better compat)
	response := "\x1b[?62;4c"
	_, _ = t.ptyFile.WriteString(response)
}

// respondToDA2 sends secondary device attributes response.
// Response: ESC [ > Pp ; Pv ; Pc c
// Pp=1 (VT220), Pv=0 (firmware version), Pc=0 (ROM cartridge)
func (t *Terminal) respondToDA2() {
	// Identify as VT220, version 0
	response := "\x1b[>1;0;0c"
	_, _ = t.ptyFile.WriteString(response)
}

// respondToWindowSizePixels responds to XTWINOPS 14 (window size in pixels).
// Response: ESC [ 4 ; height ; width t
func (t *Terminal) respondToWindowSizePixels() {
	t.mu.Lock()
	rows := t.rows
	cols := t.cols
	t.mu.Unlock()

	// Fake pixel size: assume 8x16 character cells (common default)
	height := rows * 16
	width := cols * 8
	response := fmt.Sprintf("\x1b[4;%d;%dt", height, width)
	_, _ = t.ptyFile.WriteString(response)
}

// respondToTextAreaSize responds to XTWINOPS 18 (text area size in chars).
// Response: ESC [ 8 ; rows ; cols t
func (t *Terminal) respondToTextAreaSize() {
	t.mu.Lock()
	rows := t.rows
	cols := t.cols
	t.mu.Unlock()

	response := fmt.Sprintf("\x1b[8;%d;%dt", rows, cols)
	_, _ = t.ptyFile.WriteString(response)
}

// respondToScreenSize responds to XTWINOPS 19 (screen size in chars).
// Response: ESC [ 9 ; rows ; cols t
func (t *Terminal) respondToScreenSize() {
	t.mu.Lock()
	rows := t.rows
	cols := t.cols
	t.mu.Unlock()

	response := fmt.Sprintf("\x1b[9;%d;%dt", rows, cols)
	_, _ = t.ptyFile.WriteString(response)
}

// respondToBackgroundColorQuery sends a response for OSC 11 query.
// Response format: ESC ] 11 ; rgb:RRRR/GGGG/BBBB ST
func (t *Terminal) respondToBackgroundColorQuery() {
	// Return black background (common default)
	response := "\x1b]11;rgb:0000/0000/0000\x1b\\"
	_, _ = t.ptyFile.WriteString(response)
}

// respondToForegroundColorQuery sends a response for OSC 10 query.
// Response format: ESC ] 10 ; rgb:RRRR/GGGG/BBBB ST
func (t *Terminal) respondToForegroundColorQuery() {
	// Return white foreground (common default)
	response := "\x1b]10;rgb:ffff/ffff/ffff\x1b\\"
	_, _ = t.ptyFile.WriteString(response)
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
