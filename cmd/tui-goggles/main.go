// tui-goggles captures text-based screenshots of TUI applications.
//
// This tool allows LLMs and other automated systems to "see" what a TUI
// application looks like by running it in a virtual terminal and capturing
// the rendered output as a clean text grid.
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
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/your-username/tui-goggles/internal/terminal"
)

type config struct {
	cols          int
	rows          int
	delay         time.Duration
	stableTimeout time.Duration
	stableTime    time.Duration
	waitForText   string
	keys          string
	outputFormat  string
	timeout       time.Duration
}

func main() {
	cfg := parseFlags()

	// Find command separator
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		fmt.Fprintln(os.Stderr, "Usage: tui-goggles [flags] -- command [args...]")
		os.Exit(1)
	}

	command := args[0]
	cmdArgs := args[1:]

	// Run the capture
	result, err := capture(command, cmdArgs, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Output result
	switch cfg.outputFormat {
	case "json":
		outputJSON(result)
	case "text":
		fmt.Print(result.Screen)
	default:
		fmt.Print(result.Screen)
	}
}

func parseFlags() config {
	cfg := config{}

	flag.IntVar(&cfg.cols, "cols", 80, "Terminal width in columns")
	flag.IntVar(&cfg.rows, "rows", 24, "Terminal height in rows")
	flag.DurationVar(&cfg.delay, "delay", 500*time.Millisecond, "Initial delay before first capture")
	flag.DurationVar(&cfg.stableTimeout, "stable-timeout", 5*time.Second, "Timeout waiting for stable screen")
	flag.DurationVar(&cfg.stableTime, "stable-time", 200*time.Millisecond, "Duration screen must be stable")
	flag.StringVar(&cfg.waitForText, "wait-for", "", "Wait for this text to appear before capturing")
	flag.StringVar(&cfg.keys, "keys", "", "Keys to send (space-separated: 'down down enter' or literal: 'hello')")
	flag.StringVar(&cfg.outputFormat, "format", "text", "Output format: text, json")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "Overall timeout for the operation")

	flag.Parse()

	return cfg
}

// CaptureResult contains the captured screenshot and metadata.
type CaptureResult struct {
	Screen    string    `json:"screen"`
	Cols      int       `json:"cols"`
	Rows      int       `json:"rows"`
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
}

func capture(command string, args []string, cfg config) (*CaptureResult, error) {
	// Create terminal
	term, err := terminal.New(command, args, terminal.Options{
		Rows: cfg.rows,
		Cols: cfg.cols,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create terminal: %w", err)
	}
	defer term.Close()

	// Set up overall timeout
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(cfg.timeout):
			term.Close()
		case <-done:
		}
	}()
	defer close(done)

	// Initial delay to let the TUI render
	time.Sleep(cfg.delay)

	// Wait for specific text if requested
	if cfg.waitForText != "" {
		err := term.WaitForText(cfg.waitForText, cfg.stableTimeout)
		if err != nil {
			return nil, fmt.Errorf("waiting for text %q: %w", cfg.waitForText, err)
		}
	}

	// Send keys if specified
	if cfg.keys != "" {
		if err := sendKeys(term, cfg.keys); err != nil {
			return nil, fmt.Errorf("sending keys: %w", err)
		}
		// Wait for screen to stabilize after key input
		time.Sleep(cfg.stableTime)
	}

	// Wait for stable screen (ignore timeout - just capture current state)
	_ = term.WaitForStable(cfg.stableTimeout, cfg.stableTime)

	// Capture screenshot
	screen := term.Screenshot()

	return &CaptureResult{
		Screen:    screen,
		Cols:      cfg.cols,
		Rows:      cfg.rows,
		Timestamp: time.Now(),
		Command:   command + " " + strings.Join(args, " "),
	}, nil
}

func sendKeys(term *terminal.Terminal, keys string) error {
	// Parse key specification
	// Supports: "down down enter" or literal strings
	parts := strings.Split(keys, " ")

	for _, part := range parts {
		key := parseKey(part)
		if err := term.SendKeys(string(key)); err != nil {
			return err
		}
		// Small delay between keys
		time.Sleep(50 * time.Millisecond)
	}

	return nil
}

func parseKey(s string) terminal.Key {
	switch strings.ToLower(s) {
	case "up":
		return terminal.KeyUp
	case "down":
		return terminal.KeyDown
	case "left":
		return terminal.KeyLeft
	case "right":
		return terminal.KeyRight
	case "enter", "return":
		return terminal.KeyEnter
	case "tab":
		return terminal.KeyTab
	case "esc", "escape":
		return terminal.KeyEscape
	case "backspace":
		return terminal.KeyBackspace
	case "delete":
		return terminal.KeyDelete
	case "home":
		return terminal.KeyHome
	case "end":
		return terminal.KeyEnd
	case "pgup", "pageup":
		return terminal.KeyPgUp
	case "pgdn", "pagedown":
		return terminal.KeyPgDn
	case "space":
		return terminal.KeySpace
	case "f1":
		return terminal.KeyF1
	case "f2":
		return terminal.KeyF2
	case "f3":
		return terminal.KeyF3
	case "f4":
		return terminal.KeyF4
	case "f5":
		return terminal.KeyF5
	case "f6":
		return terminal.KeyF6
	case "f7":
		return terminal.KeyF7
	case "f8":
		return terminal.KeyF8
	case "f9":
		return terminal.KeyF9
	case "f10":
		return terminal.KeyF10
	case "f11":
		return terminal.KeyF11
	case "f12":
		return terminal.KeyF12
	case "ctrl-a":
		return terminal.KeyCtrlA
	case "ctrl-b":
		return terminal.KeyCtrlB
	case "ctrl-c":
		return terminal.KeyCtrlC
	case "ctrl-d":
		return terminal.KeyCtrlD
	case "ctrl-e":
		return terminal.KeyCtrlE
	case "ctrl-f":
		return terminal.KeyCtrlF
	case "ctrl-g":
		return terminal.KeyCtrlG
	case "ctrl-h":
		return terminal.KeyCtrlH
	case "ctrl-i":
		return terminal.KeyCtrlI
	case "ctrl-j":
		return terminal.KeyCtrlJ
	case "ctrl-k":
		return terminal.KeyCtrlK
	case "ctrl-l":
		return terminal.KeyCtrlL
	case "ctrl-n":
		return terminal.KeyCtrlN
	case "ctrl-o":
		return terminal.KeyCtrlO
	case "ctrl-p":
		return terminal.KeyCtrlP
	case "ctrl-q":
		return terminal.KeyCtrlQ
	case "ctrl-r":
		return terminal.KeyCtrlR
	case "ctrl-s":
		return terminal.KeyCtrlS
	case "ctrl-t":
		return terminal.KeyCtrlT
	case "ctrl-u":
		return terminal.KeyCtrlU
	case "ctrl-v":
		return terminal.KeyCtrlV
	case "ctrl-w":
		return terminal.KeyCtrlW
	case "ctrl-x":
		return terminal.KeyCtrlX
	case "ctrl-y":
		return terminal.KeyCtrlY
	case "ctrl-z":
		return terminal.KeyCtrlZ
	default:
		// Treat as literal string
		return terminal.Key(s)
	}
}

func outputJSON(result *CaptureResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}
