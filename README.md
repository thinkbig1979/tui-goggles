# TUI Goggles

[![Developed with AI: Claude Code](https://img.shields.io/badge/Developed%20with%20AI-Claude%20Code-D97757)](https://claude.com/claude-code)
[![CI](https://github.com/thinkbig1979/tui-goggles/actions/workflows/ci.yml/badge.svg)](https://github.com/thinkbig1979/tui-goggles/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/thinkbig1979/tui-goggles)](https://github.com/thinkbig1979/tui-goggles/releases/latest)

A tool that allows LLMs and automated systems to "see" TUI (Text User Interface) applications by capturing their rendered output as clean text grids, and to drive them with keyboard, mouse, paste and resize input, check colors and styles, and run multi-step test scripts.

## Problem Solved

When LLMs run TUI applications, they receive raw ANSI escape sequences that are difficult to interpret. This tool spawns commands in a virtual terminal, processes all escape sequences, and returns a clean text representation of what would appear on screen.

## Installation

### As a Claude Code skill

Installs `SKILL.md` and the binary for your platform (Linux or macOS, amd64 or arm64) from the latest GitHub release into `~/.claude/skills/tui-capture/`, verifying checksums:

```bash
curl -fsSL https://raw.githubusercontent.com/thinkbig1979/tui-goggles/main/install.sh | sh
```

Set `VERSION=v0.2.0` to pin a release or `SKILL_DIR=...` to install elsewhere. Run it again to update. Each release also has a `tui-capture-skill_<os>_<arch>.zip` with the same contents.

### Binary only

With Go 1.21 or later:

```bash
go install github.com/thinkbig1979/tui-goggles/cmd/tui-goggles@latest
```

Or download `tui-goggles_<os>_<arch>` from the [releases page](https://github.com/thinkbig1979/tui-goggles/releases), or build from a checkout:

```bash
go build -o bin/tui-goggles ./cmd/tui-goggles
```

### Releasing

Push a version tag; the release workflow runs the tests, builds all platforms with `scripts/package.sh` and publishes the binaries, skill zips, `SKILL.md` and `checksums.txt` as release assets:

```bash
git tag v0.2.0 && git push origin v0.2.0
```

`scripts/package.sh v0.0.0-test /tmp/out` builds the same artifacts locally. Nothing built is committed to the repo.

## Usage

```bash
tui-goggles [flags] -- command [args...]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - capture completed, all assertions passed |
| 1 | General error - invalid arguments, command failed to start |
| 2 | Timeout - operation exceeded `-timeout`, or a script `wait-for`/`wait-gone` timed out |
| 3 | Assertion failed - an `-assert`, `-assert-style` or script `assert`/`assert-not`/`assert-style` did not hold |
| 4 | Command error - target command exited on its own with non-zero status before the capture finished |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-cols` | 80 | Terminal width in columns |
| `-rows` | 24 | Terminal height in rows |
| `-delay` | 500ms | Initial delay before capture |
| `-stable-timeout` | 5s | Timeout waiting for stable screen |
| `-stable-time` | 200ms | Duration screen must be stable |
| `-wait-for` | "" | Wait for this text to appear before capturing |
| `-wait-stable` | false | Wait for screen to stabilize before capturing |
| `-keys` | "" | Keys, mouse actions, text, pastes and resizes to send (space-separated tokens, see below) |
| `-keys-stdin` | false | Read keys from stdin (one per line) |
| `-input-delay` | 50ms | Delay between keystrokes |
| `-format` | text | Output format: `text` or `json` |
| `-output` | "" | Write output to file instead of stdout |
| `-timeout` | 30s | Overall timeout for the operation |
| `-assert` | | Assert text appears on screen (repeatable, exit 3 if not found) |
| `-check` | | Check if text appears (repeatable, adds to JSON output, no exit change) |
| `-capture-each` | false | Capture screen after each key (returns array in JSON mode) |
| `-trim` | false | Trim trailing blank lines from output |
| `-quiet` | false | Suppress output on success (useful with `-assert`) |
| `-env` | | Set environment variable (format: KEY=VALUE, repeatable) |
| `-styles` | false | Add styled spans (colors, bold, reverse, ...) to the output |
| `-assert-style` | | Assert cell styles, e.g. `'text="Tab 2" reverse bold'` (repeatable, exit 3) |
| `-script` | "" | Run a step script (one step per line: key, type, paste, click, resize, wait-for, wait-gone, capture, assert, ...) from a file or `-` for stdin; see SKILL.md |
| `-fg` | #ffffff | Foreground color reported for OSC 10/12 queries |
| `-bg` | #000000 | Background color reported for OSC 11 queries (e.g. `#fdf6e3` to test a light theme) |
| `-grace` | 1s | On exit, time the app gets after SIGHUP before SIGKILL (0 = kill at once) |
| `-version` | | Print the version and exit |

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

# Assert expected text is present (for automated testing)
tui-goggles -assert "Welcome" -assert "Login" -- ./my-tui-app

# Quiet mode - only exit code matters (for CI/CD)
tui-goggles -assert "Ready" -quiet -- ./my-tui-app

# Check for text presence without failing (adds to JSON)
tui-goggles -check "Error" -check "Warning" -format json -- ./my-tui-app

# Capture each step of navigation (returns array of screens)
tui-goggles -keys "down enter" -capture-each -format json -- ./my-tui-app

# Get clean JSON output with cursor position and timing
tui-goggles -format json -trim -- ./my-tui-app

# Read keys from stdin for complex sequences
echo -e "down\ndown\nenter" | tui-goggles -keys-stdin -- ./my-tui-app

# Control keystroke timing for slow apps
tui-goggles -keys "down enter" -input-delay 200ms -- ./my-tui-app

# Save output to file
tui-goggles -output screenshot.txt -- ./my-tui-app

# Pass environment variables to the command
tui-goggles -env "TERM=dumb" -env "NO_COLOR=1" -- ./my-tui-app

# Wait for screen to stabilize before capturing
tui-goggles -wait-stable -- ./my-tui-app

# Use with piped input (e.g., fzf)
echo -e "apple\nbanana\ncherry" | tui-goggles -delay 500ms -- fzf

# Modified keys, mouse, typing with spaces, bracketed paste, resize
tui-goggles -keys "shift+tab alt+right click:8,0 type:'hello world' paste:\"a\nb\" resize:100x30" -- ./my-tui-app

# Check that the second tab is highlighted
tui-goggles -keys "alt+right" -assert-style 'text="Tab 2" reverse bold' -- ./my-tui-app

# Show colors and attributes alongside the text
tui-goggles -styles -- ./my-tui-app

# Test a light terminal theme
tui-goggles -bg '#fdf6e3' -- ./my-tui-app

# Run a multi-step script
tui-goggles -script steps.txt -format json -- ./my-tui-app
```

### Key Names

For the `-keys` flag, use these names (space-separated):

- **Navigation**: `up`, `down`, `left`, `right`, `home`, `end`, `pgup`/`pageup`, `pgdn`/`pagedown`, `insert`, `delete`
- **Actions**: `enter`, `tab`, `esc`, `backspace`, `space`, `backtab` (= `shift+tab`)
- **Function keys**: `f1` through `f12`
- **Modifiers**: `ctrl`, `alt`, `shift`, `meta` joined with `+` or `-`, e.g. `ctrl-a`, `shift+tab`, `alt+left`, `ctrl+pgup`, `ctrl+shift+right`, `ctrl+space` (xterm encoding, see SKILL.md); `shift+enter`, `ctrl+tab` and other combos without a legacy encoding use modifyOtherKeys when the app enables it
- **Mouse**: `click:X,Y[,button]`, `dblclick:`, `press:`, `release:`, `drag:X1,Y1-X2,Y2`, `move:`, `wheel-up:`/`wheel-down:`/`wheel-left:`/`wheel-right:`, with 0-based cells (X = column) and optional modifiers (`shift+click:3,0`), sent as SGR 1006
- **Text**: `type:"text with spaces"` types verbatim, `paste:"text"` sends a bracketed paste; quotes group, and `\t \n \r \e \s \xHH` escapes work anywhere
- **Resize**: `resize:100x30` resizes the terminal mid-session (the app gets SIGWINCH)
- **Literal text**: Any other string is sent as-is

### Scripts

`-script FILE` (or `-` for stdin) runs one step per line against a single running app:

```
wait-for Ready
key alt+right ctrl+pgdn
type hello world
paste line one\nline two
click 12,0
drag 5,3 20,3
resize 100x30
wait-gone Loading
capture after-edit
assert hello world
assert-not Error
assert-style text="Tab 2" reverse bold
```

The run stops at the first failing step and names it (line, step, error); the screen at that moment is added as a `failure` capture. See [SKILL.md](SKILL.md) for every step.

### Styles

`-styles` adds the styled regions of the screen as spans: runs of cells on one row that share a non-default style, with `fg`/`bg` (`default`, `ansi:N` or `#rrggbb`) and attributes (`bold`, `italic`, `underline`, `blink`, `reverse`). `-assert-style` (and the `assert-style` script step) checks a selection (`text=STRING`, `X,Y` or `X,Y,LEN`) against expectations such as `reverse`, `!bold`, `fg=#ff0000`, `bg=ansi:4` or `plain`.

### JSON Output Format

Single capture (`-format json`):
```json
{
  "screen": "...",
  "cols": 80,
  "rows": 24,
  "cursor_row": 0,
  "cursor_col": 0,
  "cursor_visible": true,
  "timestamp": "2024-01-15T10:30:00Z",
  "command": "my-app --flag",
  "checks": {"Login": true, "Error": false},
  "spans": [{"row": 0, "col": 7, "len": 7, "text": " Tab 2 ", "fg": "default", "bg": "default", "attrs": ["bold", "reverse"]}],
  "timing": {
    "total_ms": 1250,
    "delay_ms": 500,
    "stabilize_ms": 200,
    "wait_for_text_ms": 350,
    "keys_ms": 200
  },
  "process": {"ended_by": "hangup", "exit_code": 0}
}
```

`cols`/`rows` are the live size (after any `resize:`). `spans` appears with `-styles`. `process` says how the app ended: `exited` (on its own), `hangup` (after SIGHUP) or `killed` (SIGKILL after `-grace`), with `signal` when a signal ended it.

Multi-capture (`-format json -capture-each`):
```json
{
  "captures": [
    {"screen": "...", "cursor_row": 0, "cursor_col": 0, ...},
    {"screen": "...", "cursor_row": 1, "cursor_col": 0, ...}
  ],
  "command": "my-app --flag",
  "timing": {...},
  "process": {...}
}
```

Script run (`-format json -script ...`): captures carry `name` and `line`, plus `steps_run` and, on failure, `failed`:
```json
{
  "captures": [{"name": "after-edit", "line": 9, "screen": "...", ...}],
  "command": "my-app",
  "steps_run": 12,
  "failed": {"line": 11, "step": "assert-not Error", "error": "text \"Error\" is on screen"},
  "process": {...}
}
```

## How It Works

1. Creates a PTY (pseudo-terminal) to run the target command, with a fixed terminal environment
2. Uses `vt10x` to emulate a VT100/xterm terminal and process escape sequences
3. **Responds to terminal queries** (device attributes, mode reports, colors, version, size) so applications like Bubble Tea initialize properly, and filters sequences the emulator would misread
4. Sends input encoded the way xterm does (CSI modifier keys, SGR mouse, bracketed paste, modifyOtherKeys when enabled)
5. Captures the virtual terminal buffer as a text grid, optionally with cell styles
6. On exit, sends SIGHUP and gives the app `-grace` to quit before SIGKILL, like closing a terminal window

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
- `ESC[c` (DA1: VT220 with ANSI color) and `ESC[>c` (DA2)
- `ESC[>q` (XTVERSION: `tui-goggles`)
- `ESC[?Ps$p` / `ESC[Ps$p` (DECRQM mode reports: real state for modes the emulator implements, "not recognized" for others such as synchronized output 2026 and grapheme clustering 2027)
- `ESC[14t` / `ESC[18t` / `ESC[19t` (XTWINOPS - window/screen size)
- `ESC]10;?` / `ESC]11;?` / `ESC]12;?` (foreground/background/cursor color, set with `-fg`/`-bg`)

Queries are handled even when split across reads.

**Deliberately not answered** (the app falls back as it would on a terminal without the feature):
- Kitty keyboard protocol query, so keys stay in xterm encoding
- Palette, clipboard and other OSC queries, DECRQSS, XTGETTCAP

Sequences vt10x would misread are dropped or normalized: kitty keyboard push/pop, modifyOtherKeys, cursor style, and colon-form SGR such as `4:3` or `38:2::r:g:b`.

**Environment:** the app gets `TERM=xterm-256color` and `COLORTERM=truecolor`; variables describing the outer terminal or forcing colors (`TERM_PROGRAM`, `NO_COLOR`, `TMUX`, `KITTY_*`, ...) are not inherited, so results do not depend on where tui-goggles runs. `-env` overrides any of it.

**Other limitations:**
- Complex Unicode (combining characters, wide chars) may have issues
- Styles: bold text in colors 0-7 is reported as 8-15, RGB values below 256 appear as palette indexes, no faint or strikethrough
- Right-to-left text not fully supported
- Timing-sensitive animations may not capture correctly

### When to Use an Alternative

For apps requiring maximum compatibility (graphics, advanced terminal features), consider using something like [mcp-tui-driver](https://github.com/michaellee8/mcp-tui-server) which uses the more complete `wezterm-term` emulator

## Technical Details

### Dependencies

- `github.com/creack/pty` - PTY handling
- `github.com/hinshun/vt10x` - VT100 terminal emulation

## Development

```bash
go vet ./...
go test -race ./...
```

Tests run real PTYs (`sh`, `cat`), so they need Linux or macOS. CI runs them on both for every push and pull request. `testapps/bt2probe` is a small Bubble Tea v2 program (its own Go module) that logs every input event it receives; use it to check how an app sees tui-goggles' input:

```bash
go -C testapps/bt2probe build -o bt2probe . && tui-goggles -keys "shift+tab click:8,0" -- testapps/bt2probe/bt2probe
```

## Use Cases

- **LLM-driven testing**: Let AI assistants verify TUI application states
- **Automated testing**: Capture and compare TUI screenshots in CI/CD
- **Documentation**: Generate text-based screenshots for docs
- **Accessibility**: Convert visual TUI state to text for screen readers

## License

MIT
