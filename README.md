# TUI Goggles

[![Built with Claude](https://img.shields.io/badge/Built%20with-Claude-blue)](https://claude.ai)

A tool that allows LLMs and automated systems to "see" TUI (Text User Interface) applications by capturing their rendered output as clean text grids.

## Problem Solved

When LLMs run TUI applications, they receive raw ANSI escape sequences that are difficult to interpret. This tool spawns commands in a virtual terminal, processes all escape sequences, and returns a clean text representation of what would appear on screen.

## Installation

```bash
cd tui-goggles
go build -o bin/tui-goggles ./cmd/tui-goggles
```

## Usage

```bash
tui-goggles [flags] -- command [args...]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-cols` | 80 | Terminal width in columns |
| `-rows` | 24 | Terminal height in rows |
| `-delay` | 500ms | Initial delay before capture |
| `-stable-timeout` | 5s | Timeout waiting for stable screen |
| `-stable-time` | 200ms | Duration screen must be stable |
| `-wait-for` | "" | Wait for this text to appear before capturing |
| `-keys` | "" | Keys to send (space-separated) |
| `-format` | text | Output format: `text` or `json` |
| `-timeout` | 30s | Overall timeout for the operation |

### Examples

```bash
# Capture initial screen of a TUI app
tui-goggles -delay 1s -- ./my-tui-app

# Capture with custom terminal size
tui-goggles -cols 120 -rows 40 -- ./my-tui-app

# Navigate a menu and capture
tui-goggles -keys "down down enter" -delay 1s -- ./my-tui-app

# Wait for specific text before capturing
tui-goggles -wait-for "Main Menu" -- ./my-tui-app

# Get JSON output with metadata
tui-goggles -format json -- ./my-tui-app

# Use with piped input (e.g., fzf)
echo -e "apple\nbanana\ncherry" | tui-goggles -delay 500ms -- fzf
```

### Key Names

For the `-keys` flag, use these names (space-separated):

- **Navigation**: `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdn`
- **Actions**: `enter`, `tab`, `esc`, `backspace`, `delete`, `space`
- **Function keys**: `f1` through `f12`
- **Ctrl combinations**: `ctrl-a` through `ctrl-z`
- **Literal text**: Any other string is sent as-is

## How It Works

1. Creates a PTY (pseudo-terminal) to run the target command
2. Uses `vt10x` to emulate a VT100/xterm terminal and process escape sequences
3. **Responds to terminal queries** (DSR, DA1, OSC color queries) so applications like Bubble Tea can properly initialize
4. Captures the virtual terminal buffer as a text grid

## Compatibility

### Tested Applications

| Application | Framework | Status |
|-------------|-----------|--------|
| Bubble Tea apps | Go (charmbracelet/bubbletea) | Works |
| fzf | Go | Works |
| htop | ncurses | Works |
| top | ncurses | Works |
| nano | ncurses | Works |
| Midnight Commander | ncurses/S-Lang | Works |

### What Works Well

- **Text-based TUI apps** - menus, lists, forms, editors
- **ncurses applications** - the vast majority of terminal apps
- **Bubble Tea / bubbletea apps** - Go TUI framework
- **CLI tools with formatted output**

### Known Limitations

**Graphics protocols are not supported:**
- Sixel graphics - will show blank or escape codes
- Kitty graphics protocol - inline images won't render
- iTerm2 inline images - won't render

**Terminal queries we respond to:**
- `ESC[5n` / `ESC[6n` (DSR - device status, cursor position) - via vt10x
- `ESC[c` / `ESC[0c` (DA1 - primary device attributes)
- `ESC[>c` / `ESC[>0c` (DA2 - secondary device attributes)
- `ESC[14t` / `ESC[18t` / `ESC[19t` (XTWINOPS - window/screen size)
- `ESC]10;?` / `ESC]11;?` (foreground/background color)

**Queries we don't handle (may affect some apps):**
- Kitty keyboard protocol
- DECRQSS (request settings)
- Some advanced xterm extensions

**Other limitations:**
- Complex Unicode (combining characters, wide chars) may have issues
- Right-to-left text not fully supported
- Timing-sensitive animations may not capture correctly

### When to Use an Alternative

For apps requiring maximum compatibility (graphics, advanced terminal features), consider using something like [mcp-tui-driver](https://github.com/michaellee8/mcp-tui-server) which uses the more complete `wezterm-term` emulator

## Technical Details

### Terminal Query Handling

Many modern TUI applications (especially Bubble Tea) send queries to detect terminal capabilities:

- **DSR (Device Status Report)**: `ESC[6n` - Asks for cursor position
- **DA1 (Device Attributes)**: `ESC[c` - Asks for terminal type
- **OSC 10/11**: Background/foreground color queries

This tool intercepts these queries and responds appropriately, allowing applications to complete their initialization and render properly.

### Dependencies

- `github.com/creack/pty` - PTY handling
- `github.com/hinshun/vt10x` - VT100 terminal emulation

## Use Cases

- **LLM-driven testing**: Let AI assistants verify TUI application states
- **Automated testing**: Capture and compare TUI screenshots in CI/CD
- **Documentation**: Generate text-based screenshots for docs
- **Accessibility**: Convert visual TUI state to text for screen readers

## License

MIT
