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
| 4 | Command error - target command exited with non-zero status |

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
  }
}
```

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

Use `-keys` with space-separated key names:

```bash
# Navigate down twice and press enter
~/.claude/skills/tui-capture/bin/tui-goggles -keys "down down enter" -- ./my-tui-app

# Type literal text
~/.claude/skills/tui-capture/bin/tui-goggles -keys "h e l l o" -- ./my-tui-app

# Read complex sequences from stdin
echo -e "down\ndown\nenter" | ~/.claude/skills/tui-capture/bin/tui-goggles -keys-stdin -- ./app
```

**Key names:**
- Navigation: `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdn`
- Actions: `enter`, `tab`, `esc`, `backspace`, `delete`, `space`
- Function keys: `f1` through `f12`
- Ctrl combos: `ctrl-a` through `ctrl-z`
- Literal characters: any single character

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
