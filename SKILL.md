---
name: tui-capture
description: Captures TUI (Text User Interface) application screens as clean text. Use when needing to debug TUI apps, test TUI output, verify terminal app state, capture what a CLI tool displays, inspect interactive terminal applications, or see rendered TUI screens. Triggers on "debug tui", "test tui", "capture tui", "tui screenshot", "terminal app output", or "see what [app] shows".
---

# TUI Capture with tui-goggles

Capture text-based screenshots of TUI applications by running them in a virtual terminal.

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

## Quick Start

```bash
# Capture a TUI app's initial screen
tui-goggles -- ./my-tui-app

# With custom terminal size
tui-goggles -cols 120 -rows 40 -- ./my-tui-app

# Wait for specific text before capturing
tui-goggles -wait-for "Main Menu" -- ./my-tui-app
```

## Core Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-cols` | 80 | Terminal width |
| `-rows` | 24 | Terminal height |
| `-delay` | 500ms | Initial delay before capture |
| `-wait-for` | "" | Text that must appear before capture |
| `-keys` | "" | Keys to send (space-separated) |
| `-format` | text | Output: `text` or `json` |
| `-timeout` | 30s | Overall timeout |
| `-stable-timeout` | 5s | Max wait for stable screen |
| `-stable-time` | 200ms | How long screen must be unchanged |

## Sending Keys

Use `-keys` with space-separated key names:

```bash
# Navigate down twice and press enter
tui-goggles -keys "down down enter" -- ./my-tui-app

# Type literal text
tui-goggles -keys "h e l l o" -- ./my-tui-app
```

**Key names:**
- Navigation: `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdn`
- Actions: `enter`, `tab`, `esc`, `backspace`, `delete`, `space`
- Function keys: `f1` through `f12`
- Ctrl combos: `ctrl-a` through `ctrl-z`
- Literal characters: any single character

## Common Patterns

### Verify a menu appears correctly
```bash
tui-goggles -wait-for "Select option" -delay 1s -- ./menu-app
```

### Navigate and capture result
```bash
tui-goggles -keys "down down enter" -delay 500ms -- ./my-app
```

### Capture fzf selection list
```bash
echo -e "apple\nbanana\ncherry" | tui-goggles -delay 500ms -- fzf
```

### Get JSON output with metadata
```bash
tui-goggles -format json -- ./my-app
```

JSON output includes: `screen`, `cols`, `rows`, `timestamp`, `command`

### Capture system tools
```bash
tui-goggles -cols 120 -rows 40 -delay 1s -- htop
```

## Prerequisites

This skill requires `tui-goggles` to be installed. Check if available:

```bash
which tui-goggles || echo "tui-goggles not found"
```

**Install from source** (requires Go 1.21+):

```bash
git clone https://github.com/yourusername/tui-goggles.git
cd tui-goggles
go build -o /usr/local/bin/tui-goggles ./cmd/tui-goggles
```

If the binary is not in PATH, use the full path in commands.

## Use Cases

- **Debug TUI apps**: See what the app is rendering without running interactively
- **Test TUI state**: Verify that menus, forms, or lists display correctly
- **Automated testing**: Capture screenshots in CI/CD pipelines
- **Documentation**: Generate text-based screenshots for docs
