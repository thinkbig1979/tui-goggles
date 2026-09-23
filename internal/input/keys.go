// Package input encodes key names, mouse actions and pastes into the byte
// sequences an xterm-compatible terminal would send to an application.
package input

import (
	"fmt"
	"strings"
)

// Modifier bits, as used in the xterm CSI modifier parameter (param = 1 + bits).
const (
	ModShift = 1
	ModAlt   = 2
	ModCtrl  = 4
	ModMeta  = 8
)

var modNames = map[string]int{
	"shift": ModShift,
	"alt":   ModAlt,
	"ctrl":  ModCtrl,
	"meta":  ModMeta,
}

// csiKey describes a key sent as a CSI sequence.
//
// Keys with a final letter (arrows, home, end, F1-F4) are sent as
// "CSI final" (or SS3 for F1-F4) without modifiers and "CSI 1;m final" with.
// Keys with a number are sent as "CSI n ~" and "CSI n;m ~".
type csiKey struct {
	final  byte   // final byte for letter-style keys
	number int    // parameter for tilde-style keys
	plain  string // unmodified sequence
}

var csiKeys = map[string]csiKey{
	"up":     {final: 'A', plain: "\x1b[A"},
	"down":   {final: 'B', plain: "\x1b[B"},
	"right":  {final: 'C', plain: "\x1b[C"},
	"left":   {final: 'D', plain: "\x1b[D"},
	"home":   {final: 'H', plain: "\x1b[H"},
	"end":    {final: 'F', plain: "\x1b[F"},
	"f1":     {final: 'P', plain: "\x1bOP"},
	"f2":     {final: 'Q', plain: "\x1bOQ"},
	"f3":     {final: 'R', plain: "\x1bOR"},
	"f4":     {final: 'S', plain: "\x1bOS"},
	"insert": {number: 2, plain: "\x1b[2~"},
	"delete": {number: 3, plain: "\x1b[3~"},
	"pgup":   {number: 5, plain: "\x1b[5~"},
	"pgdn":   {number: 6, plain: "\x1b[6~"},
	"f5":     {number: 15, plain: "\x1b[15~"},
	"f6":     {number: 17, plain: "\x1b[17~"},
	"f7":     {number: 18, plain: "\x1b[18~"},
	"f8":     {number: 19, plain: "\x1b[19~"},
	"f9":     {number: 20, plain: "\x1b[20~"},
	"f10":    {number: 21, plain: "\x1b[21~"},
	"f11":    {number: 23, plain: "\x1b[23~"},
	"f12":    {number: 24, plain: "\x1b[24~"},
}

// keyAliases maps alternative key names to their canonical name.
var keyAliases = map[string]string{
	"pageup":    "pgup",
	"pagedown":  "pgdn",
	"pgdown":    "pgdn",
	"return":    "enter",
	"escape":    "esc",
	"del":       "delete",
	"ins":       "insert",
	"backtab":   "shift+tab",
	"arrowup":   "up",
	"arrowdown": "down",
}

// simpleKeys are keys sent as a single byte (or short sequence) without modifiers.
var simpleKeys = map[string]string{
	"enter":     "\r",
	"tab":       "\t",
	"esc":       "\x1b",
	"backspace": "\x7f",
	"space":     " ",
}

// EncodeKey encodes a key specification such as "down", "shift+tab",
// "ctrl+alt+left", "alt-x" or "ctrl-pgup" into terminal input bytes.
//
// Modifiers are shift, alt, ctrl and meta, joined to the key with '+' or '-'.
// The key is a named key or a single character.
//
// It returns ok=false when spec is not a key specification at all (no known
// modifier prefix and no known key name), so the caller can treat it as
// literal text. It returns an error when spec has modifiers but the key is
// unknown, or when the combination has no legacy xterm encoding.
func EncodeKey(spec string) (seq string, ok bool, err error) {
	mods, key, hasMods := splitModifiers(spec)
	if !hasMods {
		if canonical, found := lookupName(key); found {
			// A bare alias such as "backtab" may itself carry modifiers.
			if canonical != strings.ToLower(key) {
				return EncodeKey(canonical)
			}
			return encode(0, canonical)
		}
		return "", false, nil
	}

	if key == "" {
		return "", false, nil
	}
	canonical, found := lookupName(key)
	if !found {
		if len([]rune(key)) != 1 {
			return "", true, fmt.Errorf("unknown key %q in %q", key, spec)
		}
		canonical = key
	}
	// Aliases like "backtab" expand to their own modifiers.
	if extraMods, base, has := splitModifiers(canonical); has {
		mods |= extraMods
		canonical = base
	}
	return encode(mods, canonical)
}

// splitModifiers strips leading "mod+" / "mod-" prefixes from spec.
func splitModifiers(spec string) (mods int, key string, hasMods bool) {
	rest := spec
	for {
		idx := strings.IndexAny(rest, "+-")
		if idx <= 0 {
			break
		}
		bit, isMod := modNames[strings.ToLower(rest[:idx])]
		if !isMod {
			break
		}
		mods |= bit
		hasMods = true
		rest = rest[idx+1:]
	}
	return mods, rest, hasMods
}

// lookupName resolves a (case-insensitive) key name to its canonical form.
func lookupName(name string) (string, bool) {
	lower := strings.ToLower(name)
	if alias, ok := keyAliases[lower]; ok {
		return alias, true
	}
	if _, ok := csiKeys[lower]; ok {
		return lower, true
	}
	if _, ok := simpleKeys[lower]; ok {
		return lower, true
	}
	return "", false
}

func encode(mods int, key string) (string, bool, error) {
	if k, found := csiKeys[key]; found {
		if mods == 0 {
			return k.plain, true, nil
		}
		param := 1 + mods
		if k.number != 0 {
			return fmt.Sprintf("\x1b[%d;%d~", k.number, param), true, nil
		}
		return fmt.Sprintf("\x1b[1;%d%c", param, k.final), true, nil
	}

	if s, found := simpleKeys[key]; found {
		return encodeSimple(mods, key, s)
	}

	// Single character.
	r := []rune(key)
	if len(r) != 1 {
		return "", true, fmt.Errorf("unknown key %q", key)
	}
	return encodeChar(mods, r[0])
}

func encodeSimple(mods int, key, plain string) (string, bool, error) {
	alt := mods&(ModAlt|ModMeta) != 0
	rest := mods &^ (ModAlt | ModMeta)
	prefix := ""
	if alt {
		prefix = "\x1b"
	}

	switch {
	case rest == 0:
		return prefix + plain, true, nil
	case key == "tab" && rest == ModShift:
		if alt {
			return "\x1b\x1b[Z", true, nil
		}
		return "\x1b[Z", true, nil
	case key == "space" && rest == ModCtrl:
		return prefix + "\x00", true, nil
	case key == "space" && rest == ModShift:
		return prefix + " ", true, nil
	case key == "backspace" && rest == ModCtrl:
		return prefix + "\x08", true, nil
	}
	return "", true, unencodable(mods, key)
}

func encodeChar(mods int, c rune) (string, bool, error) {
	alt := mods&(ModAlt|ModMeta) != 0
	rest := mods &^ (ModAlt | ModMeta)
	prefix := ""
	if alt {
		prefix = "\x1b"
	}

	switch rest {
	case 0:
		return prefix + string(c), true, nil
	case ModShift:
		if c >= 'a' && c <= 'z' {
			return prefix + string(c-'a'+'A'), true, nil
		}
		if c >= 'A' && c <= 'Z' {
			return prefix + string(c), true, nil
		}
	case ModCtrl:
		if b, ok := ctrlByte(c); ok {
			return prefix + string(rune(b)), true, nil
		}
	}
	return "", true, unencodable(mods, string(c))
}

// ctrlByte returns the C0 control byte produced by ctrl+c.
func ctrlByte(c rune) (byte, bool) {
	switch {
	case c >= 'a' && c <= 'z':
		return byte(c - 'a' + 1), true
	case c >= 'A' && c <= 'Z':
		return byte(c - 'A' + 1), true
	case c == '@' || c == '2' || c == ' ':
		return 0x00, true
	case c == '[' || c == '3':
		return 0x1b, true
	case c == '\\' || c == '4':
		return 0x1c, true
	case c == ']' || c == '5':
		return 0x1d, true
	case c == '^' || c == '6':
		return 0x1e, true
	case c == '_' || c == '7' || c == '/':
		return 0x1f, true
	case c == '?' || c == '8':
		return 0x7f, true
	}
	return 0, false
}

func unencodable(mods int, key string) error {
	var names []string
	for _, n := range []string{"ctrl", "alt", "shift", "meta"} {
		if mods&modNames[n] != 0 {
			names = append(names, n)
		}
	}
	return fmt.Errorf("%s+%s has no legacy xterm encoding; send the raw sequence instead (e.g. raw:\\e[...)",
		strings.Join(names, "+"), key)
}
