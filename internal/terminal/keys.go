// Package terminal provides key constants for sending to TUI applications.
package terminal

// Key represents a keyboard key or key sequence.
type Key string

// Common key constants for TUI interaction.
const (
	// Navigation keys
	KeyUp    Key = "\x1b[A"
	KeyDown  Key = "\x1b[B"
	KeyRight Key = "\x1b[C"
	KeyLeft  Key = "\x1b[D"
	KeyHome  Key = "\x1b[H"
	KeyEnd   Key = "\x1b[F"
	KeyPgUp  Key = "\x1b[5~"
	KeyPgDn  Key = "\x1b[6~"

	// Function keys
	KeyF1  Key = "\x1bOP"
	KeyF2  Key = "\x1bOQ"
	KeyF3  Key = "\x1bOR"
	KeyF4  Key = "\x1bOS"
	KeyF5  Key = "\x1b[15~"
	KeyF6  Key = "\x1b[17~"
	KeyF7  Key = "\x1b[18~"
	KeyF8  Key = "\x1b[19~"
	KeyF9  Key = "\x1b[20~"
	KeyF10 Key = "\x1b[21~"
	KeyF11 Key = "\x1b[23~"
	KeyF12 Key = "\x1b[24~"

	// Control keys
	KeyEnter     Key = "\r"
	KeyTab       Key = "\t"
	KeyBackspace Key = "\x7f"
	KeyEscape    Key = "\x1b"
	KeyDelete    Key = "\x1b[3~"
	KeyInsert    Key = "\x1b[2~"

	// Ctrl combinations
	KeyCtrlA Key = "\x01"
	KeyCtrlB Key = "\x02"
	KeyCtrlC Key = "\x03"
	KeyCtrlD Key = "\x04"
	KeyCtrlE Key = "\x05"
	KeyCtrlF Key = "\x06"
	KeyCtrlG Key = "\x07"
	KeyCtrlH Key = "\x08"
	KeyCtrlI Key = "\x09"
	KeyCtrlJ Key = "\x0a"
	KeyCtrlK Key = "\x0b"
	KeyCtrlL Key = "\x0c"
	KeyCtrlM Key = "\x0d"
	KeyCtrlN Key = "\x0e"
	KeyCtrlO Key = "\x0f"
	KeyCtrlP Key = "\x10"
	KeyCtrlQ Key = "\x11"
	KeyCtrlR Key = "\x12"
	KeyCtrlS Key = "\x13"
	KeyCtrlT Key = "\x14"
	KeyCtrlU Key = "\x15"
	KeyCtrlV Key = "\x16"
	KeyCtrlW Key = "\x17"
	KeyCtrlX Key = "\x18"
	KeyCtrlY Key = "\x19"
	KeyCtrlZ Key = "\x1a"

	// Space
	KeySpace Key = " "
)

// Char returns a Key for a single character.
func Char(c rune) Key {
	return Key(c)
}

// String converts a string to key input.
func String(s string) Key {
	return Key(s)
}
