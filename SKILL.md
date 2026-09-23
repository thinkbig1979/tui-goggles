---
name: tui-capture
description: Captures TUI (Text User Interface) application screens as clean text. Use when needing to debug TUI apps, test TUI output, verify terminal app state, capture what a CLI tool displays, inspect interactive terminal applications, or see rendered TUI screens. Triggers on "debug tui", "test tui", "capture tui", "tui screenshot", "terminal app output", or "see what [app] shows".
---

# TUI Capture with tui-goggles

Capture text-based screenshots of TUI applications by running them in a virtual terminal.

## Binary Location

This skill bundles `tui-goggles`. Use this path in all commands:

```
~/.claude/skills/tui-capture/bin/tui-goggles
```

## Recommended Usage for Agents

**Always use JSON format with trim for structured output:**
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -format json -trim -- ./app
```

**Use -quiet with -assert for pass/fail checks (only exit code matters):**
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -assert "Expected" -quiet -- ./app
# Exit 0 = text found, Exit 3 = not found
```

**Use -check for non-fatal presence detection (adds to JSON):**
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -check "Error" -check "Success" -format json -- ./app
# Returns: {"checks": {"Error": false, "Success": true}, ...}
```

**Capture navigation sequences to see each step:**
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -keys "down enter" -capture-each -format json -- ./app
```

**Save to file for later analysis:**
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -output /tmp/screen.json -format json -- ./app
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - capture completed, all assertions passed |
| 1 | General error - invalid arguments, command failed to start |
| 2 | Timeout - operation exceeded timeout |
| 3 | Assertion failed - text from `-assert` was not found |
| 4 | Command error - target command exited on its own with non-zero status before the capture finished |

## What This Tool Does

Runs a TUI application in a virtual terminal, processes all ANSI escape sequences, and returns a clean text grid of what would appear on screen. This allows inspection of TUI state without running interactively.

## Not Supported

This tool captures **text-based** TUI output only. It does NOT support:
- **Sixel graphics** - renders blank or shows escape codes
- **Kitty graphics protocol** - inline images won't render
- **iTerm2 inline images** - won't render
- **Complex Unicode** - combining characters and wide chars may have issues
- **Right-to-left text** - not fully supported
- **Timing-sensitive animations** - may not capture correctly

For apps requiring graphics or advanced terminal features, use a full terminal emulator instead.

## Core Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-cols` | 80 | Terminal width |
| `-rows` | 24 | Terminal height |
| `-delay` | 500ms | Initial delay before capture |
| `-wait-for` | "" | Text that must appear before capture |
| `-wait-stable` | false | Wait for screen to stabilize before capture |
| `-keys` | "" | Keys to send (space-separated) |
| `-keys-stdin` | false | Read keys from stdin (one per line) |
| `-input-delay` | 50ms | Delay between keystrokes |
| `-format` | text | Output: `text` or `json` |
| `-output` | "" | Write to file instead of stdout |
| `-timeout` | 30s | Overall timeout |
| `-stable-timeout` | 5s | Max wait for stable screen |
| `-stable-time` | 200ms | How long screen must be unchanged |
| `-assert` | | Assert text appears (repeatable, exit 3 if not found) |
| `-check` | | Check text presence (repeatable, adds to JSON, no exit change) |
| `-capture-each` | false | Capture after each key (array in JSON mode) |
| `-trim` | false | Remove trailing blank lines |
| `-quiet` | false | Suppress output on success |
| `-env` | | Set env var for command (KEY=VALUE, repeatable) |
| `-grace` | 1s | On exit, time the app gets after SIGHUP before SIGKILL (0 = kill at once) |

## JSON Output Format

**Single capture:**
```json
{
  "screen": "...",
  "cols": 80,
  "rows": 24,
  "cursor_row": 0,
  "cursor_col": 0,
  "cursor_visible": true,
  "timestamp": "2024-01-15T10:30:00Z",
  "command": "my-app",
  "checks": {"Login": true, "Error": false},
  "timing": {
    "total_ms": 1250,
    "delay_ms": 500,
    "stabilize_ms": 200
  },
  "process": {"ended_by": "hangup", "exit_code": 0}
}
```

`cols`/`rows` are the live size, so they reflect any `resize:`. `process` says how the app ended: `exited` (quit on its own before the capture finished), `hangup` (quit after tui-goggles sent SIGHUP) or `killed` (SIGKILL after the `-grace` period); `signal` names the signal if one ended it. In `-capture-each` mode `process` is on the top-level object.

**Multi-capture with `-capture-each`:**
```json
{
  "captures": [
    {"screen": "...", "cursor_row": 0, "cursor_col": 0, ...},
    {"screen": "...", "cursor_row": 1, "cursor_col": 0, ...}
  ],
  "command": "my-app",
  "timing": {...}
}
```

## Sending Keys

Use `-keys` with space-separated tokens. Each token is a key specification or literal text:

```bash
# Navigate down twice and press enter
~/.claude/skills/tui-capture/bin/tui-goggles -keys "down down enter" -- ./my-tui-app

# Type literal text: a token that is not a key name is sent as-is (use type: for spaces)
~/.claude/skills/tui-capture/bin/tui-goggles -keys "hello enter type:'two words'" -- ./my-tui-app

# Modified keys
~/.claude/skills/tui-capture/bin/tui-goggles -keys "shift+tab alt+left ctrl+pgdn shift+up ctrl+home" -- ./my-tui-app

# Read complex sequences from stdin
echo -e "down\ndown\nenter" | ~/.claude/skills/tui-capture/bin/tui-goggles -keys-stdin -- ./app
```

**Key names** (case-insensitive):
- Navigation: `up`, `down`, `left`, `right`, `home`, `end`, `pgup`/`pageup`, `pgdn`/`pagedown`, `insert`, `delete`
- Actions: `enter`/`return`, `tab`, `esc`/`escape`, `backspace`, `space`, `backtab` (= `shift+tab`)
- Function keys: `f1` through `f12`
- Any single character, e.g. `x`, `/`, `?`

**Modifiers:** `ctrl`, `alt`, `shift`, `meta`, joined with `+` or `-` in any order: `ctrl+left`, `alt-x`, `ctrl+shift+right`, `shift+f5`. Keys are encoded the way xterm sends them:

| Key kind | Encoding | Examples |
|----------|----------|----------|
| Arrows, home, end, F1-F4 | `CSI 1;m X` | `alt+left` = `\e[1;3D`, `ctrl+home` = `\e[1;5H` |
| pgup, pgdn, insert, delete, F5-F12 | `CSI n;m ~` | `ctrl+pgup` = `\e[5;5~` |
| `shift+tab` | `CSI Z` | |
| `alt+<char or key>` | ESC prefix | `alt+x` = `\ex`, `alt+enter` = `\e\r` |
| `ctrl+<letter>` | C0 control byte | `ctrl+c` = `\x03`, `ctrl+space` = `\x00` |
| `shift+<letter>` | Upper-case letter | `shift+a` = `A` |

`m` = 1 + shift(1) + alt(2) + ctrl(4) + meta(8). Combinations with no legacy xterm encoding (`shift+enter`, `ctrl+tab`, `ctrl+shift+a`) are rejected with exit 1 rather than silently mis-sent. Note that `ctrl+m`, `ctrl+i` and `ctrl+[` send the same bytes as `enter`, `tab` and `esc`, so the app sees those keys.

## Typing, Pasting and Escapes

Tokens are split on spaces, so use `type:` for text with spaces and quote it. Double or single quotes group text and can sit inside a token:

| Token | Sends |
|-------|-------|
| `type:"hello world"` | the text verbatim in one write, with no key-name parsing (`type:enter` types "enter") |
| `paste:"some text"` | a bracketed paste: `\e[200~some text\e[201~` (Bubble Tea delivers one `PasteMsg`) |

Backslash escapes work inside and outside quotes: `\t` tab, `\n` LF, `\r` CR, `\e` ESC, `\s` space, `\\` `\"` `\'`, `\xHH` any byte. So `type:a\tb` types a, Tab, b, and a bare `\e[1;3D` token sends a raw alt+left. Enter is `\r` (or the `enter` key); `\n` is Ctrl+J to most apps.

```bash
# Type a line with spaces, press Enter, paste two lines
~/.claude/skills/tui-capture/bin/tui-goggles -keys "type:'hello world' enter paste:\"line one\nline two\"" -- ./editor

# With -keys-stdin, each line is tokenized separately (quotes cannot span lines)
printf 'type:"x y"\npaste:"p q" enter\n' | ~/.claude/skills/tui-capture/bin/tui-goggles -keys-stdin -- ./editor
```

A quote that is never closed, or a bad `\x` escape, fails with exit 1 before the app starts.

## Resizing and Shutdown

`resize:COLSxROWS` in `-keys` resizes the terminal mid-session: the emulator is resized in place (screen contents kept) and the app gets SIGWINCH, as in a real terminal.

```bash
# Check reflow: start at 80x24, shrink, capture
~/.claude/skills/tui-capture/bin/tui-goggles -keys "resize:60x10" -format json -- ./app
```

When the capture is done, tui-goggles ends the app the way closing a terminal window does: SIGHUP to its process group, then SIGKILL if it is still running after `-grace` (default 1s). An app that saves on SIGHUP gets to save, so you can check files it writes on exit. Use `-grace 0` for the old kill-at-once behaviour.

## Mouse

Mouse tokens go in `-keys` like any other token, as `verb:X,Y`. Coordinates are **0-based cells** (the same as `cursor_col`/`cursor_row` in JSON output: `X` is the column, `Y` the row). Events are sent as SGR 1006 sequences (`\e[<b;x;yM` / `m`), which Bubble Tea v2 and most modern TUIs expect.

| Token | Sends |
|-------|-------|
| `click:X,Y[,button]` | press + release |
| `dblclick:X,Y[,button]` | two clicks in one write, no input delay between them |
| `press:X,Y[,button]` / `release:X,Y[,button]` | a single press or release |
| `drag:X1,Y1-X2,Y2[,button]` | press at start, one motion event per cell along the line, release at end |
| `move:X,Y` | motion with no button held (needs all-motion tracking, mode 1003) |
| `wheel-up:X,Y`, `wheel-down:X,Y`, `wheel-left:X,Y`, `wheel-right:X,Y` | one wheel notch |

`button` is `left` (default), `middle` or `right`. Prefix modifiers the same way as keys: `shift+click:3,0`, `ctrl+wheel-up:10,5`, `alt+drag:0,2-8,2`.

```bash
# Click the second tab, drag-select across a line, scroll down twice
~/.claude/skills/tui-capture/bin/tui-goggles -keys "click:8,0 drag:2,5-20,5 wheel-down:10,10 wheel-down:10,10" -- ./app
```

A warning is printed to stderr (and the event is still sent) when the app has not enabled the needed mouse mode, or when the cell is outside the terminal.

## Common Patterns

### Quick pass/fail test
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -assert "Ready" -quiet -- ./app && echo "PASS" || echo "FAIL"
```

### Check for multiple conditions without failing
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -check "Error" -check "Warning" -check "Success" -format json -trim -- ./app
```

### Navigate and verify result
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -keys "down down enter" -assert "Selected" -format json -- ./my-app
```

### Capture fzf selection list
```bash
echo -e "apple\nbanana\ncherry" | ~/.claude/skills/tui-capture/bin/tui-goggles -format json -trim -- fzf
```

### Watch navigation step-by-step
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -keys "down enter" -capture-each -format json -- ./my-app
```

### Handle slow apps
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -input-delay 200ms -wait-stable -keys "down enter" -- ./slow-app
```

### Capture system tools
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -cols 120 -rows 40 -delay 1s -format json -trim -- htop
```

### Save for later analysis
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -output /tmp/state.json -format json -trim -- ./app
```

### Set environment variables
```bash
~/.claude/skills/tui-capture/bin/tui-goggles -env "NO_COLOR=1" -env "TERM=dumb" -- ./app
```

## Use Cases

- **Debug TUI apps**: See what the app is rendering without running interactively
- **Test TUI state**: Verify menus, forms, or lists display correctly
- **Automated testing**: Use `-assert -quiet` for CI/CD pass/fail checks
- **Presence detection**: Use `-check` to detect multiple conditions in JSON
- **Documentation**: Generate text-based screenshots for docs
