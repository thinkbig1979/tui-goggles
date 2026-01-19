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
	done    chan struct{}
	err     error
}

// Options configures the terminal emulator.
type Options struct {
	Rows int
	Cols int
	Env  []string
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
	}

	// Start reading from PTY and feeding to virtual terminal
	go t.readLoop()

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

// ScreenshotWithCursor captures the screen and marks cursor position.
func (t *Terminal) ScreenshotWithCursor() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Get the raw string representation
	screen := t.vt.String()

	// Optionally, we could mark the cursor position here
	// For now, just return the plain screen
	return screen
}

// SendKeys sends keystrokes to the running application.
func (t *Terminal) SendKeys(keys string) error {
	_, err := t.ptyFile.WriteString(keys)
	return err
}

// SendKey sends a single key (including special keys) to the application.
func (t *Terminal) SendKey(key Key) error {
	return t.SendKeys(string(key))
}

// Wait waits for the command to exit.
func (t *Terminal) Wait() error {
	<-t.done
	return t.cmd.Wait()
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

// Close terminates the command and cleans up resources.
func (t *Terminal) Close() error {
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	if t.ptyFile != nil {
		_ = t.ptyFile.Close()
	}
	<-t.done
	return nil
}

// Resize changes the terminal size.
func (t *Terminal) Resize(cols, rows int) error {
	// Validate dimensions to prevent overflow
	if rows < 0 || rows > maxTerminalDimension {
		return fmt.Errorf("rows must be between 0 and %d", maxTerminalDimension)
	}
	if cols < 0 || cols > maxTerminalDimension {
		return fmt.Errorf("cols must be between 0 and %d", maxTerminalDimension)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	err := pty.Setsize(t.ptyFile, &pty.Winsize{
		Rows: uint16(rows), //nolint:gosec // validated above
		Cols: uint16(cols), //nolint:gosec // validated above
	})
	if err != nil {
		return err
	}

	t.rows = rows
	t.cols = cols

	// Recreate virtual terminal with new size
	t.vt = vt10x.New(vt10x.WithSize(cols, rows))

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
	select {
	case <-t.done:
		return false
	default:
		return true
	}
}

// containsText checks if the screen contains the given text.
func containsText(screen, text string) bool {
	return text != "" && screen != "" && strings.Contains(screen, text)
}
