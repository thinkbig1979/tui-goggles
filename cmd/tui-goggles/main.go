// tui-goggles captures text-based screenshots of TUI applications.
//
// This tool allows LLMs and other automated systems to "see" what a TUI
// application looks like by running it in a virtual terminal and capturing
// the rendered output as a clean text grid.
//
// Exit codes:
//
//	0 - Success (capture completed, all assertions passed if any)
//	1 - General error (invalid arguments, command failed to start)
//	2 - Timeout (operation exceeded timeout)
//	3 - Assertion failed (text specified with -assert was not found)
//	4 - Command error (the target command exited with non-zero status)
//
// Usage:
//
//	tui-goggles [flags] -- command [args...]
//
// Examples:
//
//	# Capture initial screen after 1 second
//	tui-goggles -- ./my-tui-app
//
//	# Capture with custom terminal size
//	tui-goggles -cols 120 -rows 40 -- ./my-tui-app
//
//	# Send keys and capture result
//	tui-goggles -keys "j j enter" -- ./my-tui-app
//
//	# Wait for specific text before capturing
//	tui-goggles -wait-for "Main Menu" -- ./my-tui-app
//
//	# Assert text is present (exit 3 if not found)
//	tui-goggles -assert "Welcome" -assert "Login" -- ./my-tui-app
//
//	# Capture each state after each key
//	tui-goggles -keys "down enter" -capture-each -format json -- ./my-tui-app
//
//	# Quiet mode - only exit code matters
//	tui-goggles -assert "Ready" -quiet -- ./my-tui-app
//
//	# Read keys from stdin for complex sequences
//	echo -e "down\ndown\nenter" | tui-goggles -keys-stdin -- ./my-tui-app
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/your-username/tui-goggles/internal/input"
	"github.com/your-username/tui-goggles/internal/terminal"
)

// Exit codes
const (
	ExitSuccess         = 0
	ExitGeneralError    = 1
	ExitTimeout         = 2
	ExitAssertionFailed = 3
	ExitCommandError    = 4
)

type config struct {
	cols          int
	rows          int
	delay         time.Duration
	stableTimeout time.Duration
	stableTime    time.Duration
	waitForText   string
	keys          string
	keysStdin     bool
	outputFormat  string
	timeout       time.Duration
	asserts       []string
	checks        []string
	captureEach   bool
	trim          bool
	quiet         bool
	waitStable    bool
	outputFile    string
	envVars       []string
	inputDelay    time.Duration
	grace         time.Duration
}

// arrayFlag allows multiple flags of the same type
type arrayFlag []string

func (a *arrayFlag) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayFlag) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	cfg := parseFlags()

	// Find command separator
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		fmt.Fprintln(os.Stderr, "Usage: tui-goggles [flags] -- command [args...]")
		os.Exit(ExitGeneralError)
	}

	command := args[0]
	cmdArgs := args[1:]

	// Run the capture
	exitCode := run(command, cmdArgs, cfg)
	os.Exit(exitCode)
}

func parseFlags() config {
	cfg := config{}
	var asserts arrayFlag
	var checks arrayFlag
	var envVars arrayFlag

	flag.IntVar(&cfg.cols, "cols", 80, "Terminal width in columns")
	flag.IntVar(&cfg.rows, "rows", 24, "Terminal height in rows")
	flag.DurationVar(&cfg.delay, "delay", 500*time.Millisecond, "Initial delay before first capture")
	flag.DurationVar(&cfg.stableTimeout, "stable-timeout", 5*time.Second, "Timeout waiting for stable screen")
	flag.DurationVar(&cfg.stableTime, "stable-time", 200*time.Millisecond, "Duration screen must be stable")
	flag.StringVar(&cfg.waitForText, "wait-for", "", "Wait for this text to appear before capturing")
	flag.StringVar(&cfg.keys, "keys", "", "Keys to send, space-separated: key names ('down', 'shift+tab'), mouse ('click:3,0'), 'type:\"text\"', 'paste:\"text\"', or literal text")
	flag.BoolVar(&cfg.keysStdin, "keys-stdin", false, "Read keys from stdin (one per line)")
	flag.StringVar(&cfg.outputFormat, "format", "text", "Output format: text, json")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "Overall timeout for the operation")
	flag.Var(&asserts, "assert", "Assert this text appears on screen (can be specified multiple times, exit code 3 if not found)")
	flag.Var(&checks, "check", "Check if text appears on screen (adds to 'checks' object in JSON output, no exit code change)")
	flag.BoolVar(&cfg.captureEach, "capture-each", false, "Capture screen after each key (returns array in JSON mode)")
	flag.BoolVar(&cfg.trim, "trim", false, "Trim trailing blank lines from output")
	flag.BoolVar(&cfg.quiet, "quiet", false, "Suppress output on success (useful with -assert)")
	flag.BoolVar(&cfg.waitStable, "wait-stable", false, "Wait for screen to stabilize before capturing")
	flag.StringVar(&cfg.outputFile, "output", "", "Write output to file instead of stdout")
	flag.Var(&envVars, "env", "Set environment variable for command (format: KEY=VALUE, can be repeated)")
	flag.DurationVar(&cfg.inputDelay, "input-delay", 50*time.Millisecond, "Delay between keystrokes")
	flag.DurationVar(&cfg.grace, "grace", time.Second, "On exit, time the app gets to quit after SIGHUP before SIGKILL (0 = kill immediately)")

	flag.Parse()

	cfg.asserts = asserts
	cfg.checks = checks
	cfg.envVars = envVars
	return cfg
}

// CaptureResult contains the captured screenshot and metadata.
type CaptureResult struct {
	Screen        string          `json:"screen"`
	Cols          int             `json:"cols"`
	Rows          int             `json:"rows"`
	CursorRow     int             `json:"cursor_row"`
	CursorCol     int             `json:"cursor_col"`
	CursorVisible bool            `json:"cursor_visible"`
	Timestamp     time.Time       `json:"timestamp"`
	Command       string          `json:"command"`
	Checks        map[string]bool `json:"checks,omitempty"`
	Timing        *TimingInfo     `json:"timing,omitempty"`
	// Process describes how the app ended (final capture only).
	Process *terminal.ExitStatus `json:"process,omitempty"`
}

// TimingInfo contains timing information about the capture.
type TimingInfo struct {
	TotalMs       int64 `json:"total_ms"`
	DelayMs       int64 `json:"delay_ms"`
	StabilizeMs   int64 `json:"stabilize_ms,omitempty"`
	WaitForTextMs int64 `json:"wait_for_text_ms,omitempty"`
	KeysMs        int64 `json:"keys_ms,omitempty"`
}

// MultiCaptureResult contains multiple captures (for -capture-each mode).
type MultiCaptureResult struct {
	Captures []CaptureResult      `json:"captures"`
	Command  string               `json:"command"`
	Timing   *TimingInfo          `json:"timing,omitempty"`
	Process  *terminal.ExitStatus `json:"process,omitempty"`
}

func run(command string, args []string, cfg config) int {
	startTime := time.Now()
	timing := &TimingInfo{}

	// Parse all key tokens up front so syntax errors fail before the app starts
	actions, err := parseActions(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitGeneralError
	}

	// Create terminal with environment variables
	termOpts := terminal.Options{
		Rows:  cfg.rows,
		Cols:  cfg.cols,
		Env:   cfg.envVars,
		Grace: cfg.grace,
	}

	term, err := terminal.New(command, args, termOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create terminal: %v\n", err)
		return ExitGeneralError
	}
	defer term.Close()

	// Set up overall timeout
	timedOut := false
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(cfg.timeout):
			timedOut = true
			term.Close()
		case <-done:
		}
	}()
	defer close(done)

	// Initial delay to let the TUI render
	delayStart := time.Now()
	time.Sleep(cfg.delay)
	timing.DelayMs = time.Since(delayStart).Milliseconds()

	// Wait for specific text if requested
	if cfg.waitForText != "" {
		waitStart := time.Now()
		err := term.WaitForText(cfg.waitForText, cfg.stableTimeout)
		timing.WaitForTextMs = time.Since(waitStart).Milliseconds()
		if err != nil {
			if timedOut {
				fmt.Fprintf(os.Stderr, "Error: timeout waiting for text %q\n", cfg.waitForText)
				return ExitTimeout
			}
			fmt.Fprintf(os.Stderr, "Error: waiting for text %q: %v\n", cfg.waitForText, err)
			return ExitGeneralError
		}
	}

	// Wait for stable screen if requested (before any keys)
	if cfg.waitStable {
		stabilizeStart := time.Now()
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
		timing.StabilizeMs = time.Since(stabilizeStart).Milliseconds()
	}

	var results []CaptureResult

	// Capture initial state if capture-each mode
	if cfg.captureEach {
		results = append(results, captureScreen(term, command, args, cfg, nil))
	}

	// Send keys if specified
	if len(actions) > 0 {
		keysStart := time.Now()
		for _, action := range actions {
			if err := sendAction(term, action); err != nil {
				fmt.Fprintf(os.Stderr, "Error: sending %q: %v\n", action.Token, err)
				return ExitGeneralError
			}
			time.Sleep(cfg.inputDelay)
			if cfg.captureEach {
				// Wait for screen to stabilize after each input and capture it
				_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
				results = append(results, captureScreen(term, command, args, cfg, nil))
			}
		}
		if !cfg.captureEach {
			// Wait for screen to stabilize after key input
			time.Sleep(cfg.stableTime)
		}
		timing.KeysMs = time.Since(keysStart).Milliseconds()
	}

	// Wait for stable screen (ignore timeout - just capture current state)
	if !cfg.waitStable {
		stabilizeStart := time.Now()
		_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)
		timing.StabilizeMs = time.Since(stabilizeStart).Milliseconds()
	}

	timing.TotalMs = time.Since(startTime).Milliseconds()

	// Final capture (or only capture if not capture-each mode)
	var finalResult CaptureResult
	if cfg.captureEach {
		// Already captured, use last result
		if len(results) > 0 {
			finalResult = results[len(results)-1]
		}
	} else {
		finalResult = captureScreen(term, command, args, cfg, timing)
	}

	// Process checks (non-fatal text presence checks)
	if len(cfg.checks) > 0 {
		checksResult := make(map[string]bool)
		screen := finalResult.Screen
		for _, checkText := range cfg.checks {
			checksResult[checkText] = strings.Contains(screen, checkText)
		}
		finalResult.Checks = checksResult
		// Update checks in all results if capture-each mode
		for i := range results {
			results[i].Checks = checksResult
		}
	}

	// End the app the way closing a terminal would (SIGHUP, then SIGKILL after
	// the grace period) and record how it ended.
	exitedOnItsOwn := term.Exited()
	term.Close()
	status := term.ExitStatus()
	finalResult.Process = &status

	// Check assertions against final screen
	if len(cfg.asserts) > 0 {
		screen := finalResult.Screen
		for _, assertText := range cfg.asserts {
			if !strings.Contains(screen, assertText) {
				fmt.Fprintf(os.Stderr, "Assertion failed: text %q not found on screen\n", assertText)
				// Still output the screen for debugging (unless quiet)
				if !cfg.quiet {
					outputResult(finalResult, results, cfg, timing)
				}
				return ExitAssertionFailed
			}
		}
	}

	// Output result (unless quiet mode)
	if !cfg.quiet {
		outputResult(finalResult, results, cfg, timing)
	}

	if timedOut {
		return ExitTimeout
	}
	if exitedOnItsOwn && status.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "Error: command exited with status %d before capture finished\n", status.ExitCode)
		return ExitCommandError
	}

	return ExitSuccess
}

// parseActions tokenizes and parses -keys and, with -keys-stdin, each line
// of stdin.
func parseActions(cfg config) ([]input.Action, error) {
	var tokens []string
	if cfg.keys != "" {
		t, err := input.Tokenize(cfg.keys)
		if err != nil {
			return nil, fmt.Errorf("-keys: %w", err)
		}
		tokens = append(tokens, t...)
	}
	if cfg.keysStdin {
		scanner := bufio.NewScanner(os.Stdin)
		for line := 1; scanner.Scan(); line++ {
			t, err := input.Tokenize(scanner.Text())
			if err != nil {
				return nil, fmt.Errorf("stdin line %d: %w", line, err)
			}
			tokens = append(tokens, t...)
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading keys from stdin: %w", err)
		}
	}

	actions := make([]input.Action, 0, len(tokens))
	for _, tok := range tokens {
		a, err := input.ParseToken(tok)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", tok, err)
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func captureScreen(term *terminal.Terminal, command string, args []string, cfg config, timing *TimingInfo) CaptureResult {
	screen, cursorCol, cursorRow, cursorVisible := term.ScreenshotWithCursor()
	cols, rows := term.Size()

	if cfg.trim {
		screen = trimTrailingBlankLines(screen)
	}

	return CaptureResult{
		Screen:        screen,
		Cols:          cols,
		Rows:          rows,
		CursorCol:     cursorCol,
		CursorRow:     cursorRow,
		CursorVisible: cursorVisible,
		Timestamp:     time.Now(),
		Command:       command + " " + strings.Join(args, " "),
		Timing:        timing,
	}
}

func trimTrailingBlankLines(s string) string {
	lines := strings.Split(s, "\n")

	// Find last non-blank line
	lastNonBlank := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastNonBlank = i
			break
		}
	}

	if lastNonBlank == -1 {
		return ""
	}

	return strings.Join(lines[:lastNonBlank+1], "\n")
}

func outputResult(result CaptureResult, multiResults []CaptureResult, cfg config, timing *TimingInfo) {
	var output string

	switch cfg.outputFormat {
	case "json":
		if cfg.captureEach && len(multiResults) > 0 {
			output = formatMultiJSON(multiResults, result.Command, timing, result.Process)
		} else {
			output = formatJSON(result)
		}
	case "text":
		if cfg.captureEach && len(multiResults) > 0 {
			// For text mode with capture-each, show all captures separated by markers
			var sb strings.Builder
			for i, r := range multiResults {
				if i > 0 {
					sb.WriteString("\n--- Capture ")
					sb.WriteString(fmt.Sprintf("%d", i))
					sb.WriteString(" ---\n")
				}
				sb.WriteString(r.Screen)
			}
			output = sb.String()
		} else {
			output = result.Screen
		}
	default:
		output = result.Screen
	}

	// Write to file or stdout
	if cfg.outputFile != "" {
		err := os.WriteFile(cfg.outputFile, []byte(output), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: writing to file %q: %v\n", cfg.outputFile, err)
		}
	} else {
		fmt.Print(output)
	}
}

// sendAction sends one parsed -keys token to the application.
func sendAction(term *terminal.Terminal, action input.Action) error {
	switch action.Kind {
	case input.ActionMouse:
		warnMouseMode(term, action)
		return term.SendKeys(action.Mouse.Seq)
	case input.ActionResize:
		return term.Resize(action.Cols, action.Rows)
	default:
		return term.SendKeys(action.Bytes)
	}
}

// warnMouseMode prints a warning when the application has not enabled the
// mouse reporting a mouse action needs. The action is still sent.
func warnMouseMode(term *terminal.Terminal, action input.Action) {
	modes := term.MouseModes()
	cols, rows := term.Size()
	m := action.Mouse
	switch {
	case !modes.Buttons:
		fmt.Fprintf(os.Stderr, "Warning: %s: the app has not enabled mouse tracking (DECSET 1000/1002/1003)\n", action.Token)
	case !modes.SGR:
		fmt.Fprintf(os.Stderr, "Warning: %s: the app has not enabled SGR mouse mode (DECSET 1006); events are sent as SGR anyway\n", action.Token)
	case m.NeedsAnyMotion && !modes.AnyMotion:
		fmt.Fprintf(os.Stderr, "Warning: %s: motion without a button needs all-motion tracking (DECSET 1003)\n", action.Token)
	case m.NeedsButtonMotion && !modes.ButtonMotion:
		fmt.Fprintf(os.Stderr, "Warning: %s: drag motion needs button-motion tracking (DECSET 1002 or 1003)\n", action.Token)
	}
	if m.X >= cols || m.Y >= rows {
		fmt.Fprintf(os.Stderr, "Warning: %s: cell %d,%d is outside the %dx%d terminal (coordinates are 0-based)\n", action.Token, m.X, m.Y, cols, rows)
	}
}

func formatJSON(result CaptureResult) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
	return buf.String()
}

func formatMultiJSON(results []CaptureResult, command string, timing *TimingInfo, process *terminal.ExitStatus) string {
	multi := MultiCaptureResult{
		Captures: results,
		Command:  command,
		Timing:   timing,
		Process:  process,
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(multi)
	return buf.String()
}
