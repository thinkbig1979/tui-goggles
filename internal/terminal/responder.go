package terminal

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// maxPending caps how many bytes of an unfinished escape sequence are held
// back waiting for the next read. Longer sequences are passed through as-is.
const maxPending = 4096

// responder sits between the PTY and the vt10x emulator. It answers the
// terminal queries applications send (device attributes, mode reports,
// colors, version, size), tracks the modes the application sets, and drops
// sequences vt10x would misinterpret. Escape sequences split across reads
// are held back until complete.
type responder struct {
	w       io.Writer // replies go here (the PTY)
	writeMu *sync.Mutex
	size    func() (cols, rows int)

	fg, bg string // OSC 10/11 replies, as rgb:rrrr/gggg/bbbb

	pending []byte

	mu           sync.Mutex
	privateModes map[int]bool // DEC private modes (CSI ? Pm h/l)
	ansiModes    map[int]bool // ANSI modes (CSI Pm h/l)
}

// DEC private modes the emulator implements, with their initial state.
// Mode reports (DECRQM) for these return set/reset; any other mode is
// reported as not recognized, so applications don't enable features
// (synchronized output, grapheme clustering, in-band resize) that the
// emulator does not support.
var knownPrivateModes = map[int]bool{
	1:    false, // DECCKM application cursor keys
	5:    false, // DECSCNM reverse video
	6:    false, // DECOM origin mode
	7:    true,  // DECAWM auto-wrap
	9:    false, // X10 mouse
	25:   true,  // DECTCEM cursor visible
	47:   false, // alternate screen
	1000: false, // mouse button tracking
	1002: false, // mouse button-motion tracking
	1003: false, // mouse all-motion tracking
	1004: false, // focus reporting
	1006: false, // SGR mouse encoding
	1047: false, // alternate screen
	1049: false, // alternate screen with cursor save
	2004: false, // bracketed paste
}

var knownANSIModes = map[int]bool{
	4:  false, // IRM insert mode
	20: false, // LNM line feed/new line
}

func newResponder(w io.Writer, writeMu *sync.Mutex, size func() (int, int), fg, bg string) *responder {
	r := &responder{w: w, writeMu: writeMu, size: size, fg: fg, bg: bg}
	r.resetModes()
	return r
}

func (r *responder) resetModes() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.privateModes = make(map[int]bool, len(knownPrivateModes))
	for m, v := range knownPrivateModes {
		r.privateModes[m] = v
	}
	r.ansiModes = make(map[int]bool, len(knownANSIModes))
	for m, v := range knownANSIModes {
		r.ansiModes[m] = v
	}
}

// PrivateMode reports whether DEC private mode m is set.
func (r *responder) PrivateMode(m int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.privateModes[m]
}

// process handles a chunk of application output and returns the bytes to
// pass on to the emulator.
func (r *responder) process(chunk []byte) []byte {
	data := chunk
	if len(r.pending) > 0 {
		data = append(r.pending, chunk...)
		r.pending = nil
	}

	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if data[i] != 0x1b {
			out = append(out, data[i])
			i++
			continue
		}
		if i+1 >= len(data) {
			r.hold(data[i:], &out)
			break
		}
		switch data[i+1] {
		case '[':
			end := scanCSI(data, i+2)
			if end == csiIncomplete {
				r.hold(data[i:], &out)
				return out
			}
			if end == csiMalformed {
				out = append(out, data[i])
				i++
				continue
			}
			out = append(out, r.handleCSI(data[i:end])...)
			i = end
		case ']':
			end, termLen := scanOSC(data, i+2)
			if end < 0 {
				r.hold(data[i:], &out)
				return out
			}
			out = append(out, r.handleOSC(data[i:end], data[end-termLen:end])...)
			i = end
		case 'c': // RIS full reset
			r.resetModes()
			out = append(out, data[i:i+2]...)
			i += 2
		default:
			out = append(out, data[i])
			i++
		}
	}
	return out
}

// hold keeps an unfinished sequence for the next chunk, or passes it on if
// it is implausibly long.
func (r *responder) hold(seq []byte, out *[]byte) {
	if len(seq) > maxPending {
		*out = append(*out, seq...)
		return
	}
	r.pending = append([]byte(nil), seq...)
}

// flush returns any held-back bytes (used when the PTY closes).
func (r *responder) flush() []byte {
	p := r.pending
	r.pending = nil
	return p
}

const (
	csiIncomplete = -1
	csiMalformed  = -2
)

// scanCSI returns the index just past the final byte of the CSI sequence
// whose parameters start at data[start], csiIncomplete if the data ends
// first, or csiMalformed if a byte that cannot be in a CSI sequence occurs.
func scanCSI(data []byte, start int) int {
	for j := start; j < len(data); j++ {
		b := data[j]
		switch {
		case b >= 0x40 && b <= 0x7e:
			return j + 1
		case b >= 0x20 && b <= 0x3f:
			continue
		default:
			return csiMalformed
		}
	}
	return csiIncomplete
}

// scanOSC returns the index just past the terminator (BEL or ESC \) of the
// OSC sequence whose payload starts at data[start] and the terminator's
// length, or -1 if it is incomplete.
func scanOSC(data []byte, start int) (int, int) {
	for j := start; j < len(data); j++ {
		if data[j] == 0x07 {
			return j + 1, 1
		}
		if data[j] == 0x1b {
			if j+1 >= len(data) {
				return -1, 0
			}
			if data[j+1] == '\\' {
				return j + 2, 2
			}
		}
	}
	return -1, 0
}

// handleCSI answers or filters one complete CSI sequence and returns what
// to pass on to the emulator.
func (r *responder) handleCSI(seq []byte) []byte {
	body := string(seq[2 : len(seq)-1])
	final := seq[len(seq)-1]

	var prefix byte
	if len(body) > 0 && strings.IndexByte("<=>?", body[0]) >= 0 {
		prefix = body[0]
		body = body[1:]
	}
	params := body
	intermediates := ""
	if k := strings.IndexFunc(body, func(c rune) bool { return c >= 0x20 && c <= 0x2f }); k >= 0 {
		params, intermediates = body[:k], body[k:]
	}

	switch {
	// DECRQM: request mode (CSI ? Ps $ p / CSI Ps $ p)
	case final == 'p' && intermediates == "$":
		r.reportMode(prefix == '?', params)
		return nil

	// Primary device attributes
	case final == 'c' && prefix == 0 && (params == "" || params == "0"):
		r.reply("\x1b[?62;22c") // VT220 with ANSI color
		return nil

	// Secondary device attributes
	case final == 'c' && prefix == '>':
		r.reply("\x1b[>1;0;0c")
		return nil

	// XTVERSION
	case final == 'q' && prefix == '>' && (params == "" || params == "0"):
		r.reply("\x1bP>|tui-goggles\x1b\\")
		return nil

	// XTWINOPS size reports
	case final == 't' && prefix == 0 && intermediates == "":
		if r.reportSize(params) {
			return nil
		}
		return seq

	// Mode set/reset: track, then pass on for the emulator.
	case (final == 'h' || final == 'l') && intermediates == "" && (prefix == 0 || prefix == '?'):
		r.trackModes(prefix == '?', params, final == 'h')
		return seq

	// SGR: normalize colon sub-parameters and drop what vt10x misparses.
	case final == 'm' && prefix == 0 && intermediates == "":
		return []byte("\x1b[" + normalizeSGR(params) + "m")
	}

	// vt10x only understands plain and '?' sequences without intermediates;
	// it would mistake others (kitty keyboard CSI > u / CSI < u / CSI ? u,
	// modifyOtherKeys CSI > 4 m, cursor style CSI SP q, ...) for unrelated
	// commands such as restore-cursor or SGR reset. Drop them.
	if prefix == '<' || prefix == '=' || prefix == '>' || intermediates != "" {
		return nil
	}
	if prefix == '?' && (final == 'u' || final == 'm') {
		return nil
	}
	return seq
}

// reportMode answers a DECRQM query.
func (r *responder) reportMode(private bool, params string) {
	mode, err := strconv.Atoi(params)
	if err != nil {
		return
	}
	r.mu.Lock()
	modes := r.ansiModes
	if private {
		modes = r.privateModes
	}
	set, known := modes[mode]
	r.mu.Unlock()

	value := 0 // not recognized
	if known {
		value = 2 // reset
		if set {
			value = 1 // set
		}
	}
	q := ""
	if private {
		q = "?"
	}
	r.reply(fmt.Sprintf("\x1b[%s%d;%d$y", q, mode, value))
}

// reportSize answers XTWINOPS 14/18/19; it returns false for other ops.
func (r *responder) reportSize(params string) bool {
	cols, rows := r.size()
	switch params {
	case "14": // text area in pixels, assuming 8x16 cells
		r.reply(fmt.Sprintf("\x1b[4;%d;%dt", rows*16, cols*8))
	case "18": // text area in characters
		r.reply(fmt.Sprintf("\x1b[8;%d;%dt", rows, cols))
	case "19": // screen in characters
		r.reply(fmt.Sprintf("\x1b[9;%d;%dt", rows, cols))
	default:
		return false
	}
	return true
}

func (r *responder) trackModes(private bool, params string, set bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	modes := r.ansiModes
	if private {
		modes = r.privateModes
	}
	for _, p := range strings.Split(params, ";") {
		if m, err := strconv.Atoi(p); err == nil {
			if _, known := modes[m]; known {
				modes[m] = set
			}
		}
	}
}

// handleOSC answers color queries and passes everything else on.
func (r *responder) handleOSC(seq, terminator []byte) []byte {
	payload := string(seq[2 : len(seq)-len(terminator)])
	if !strings.HasSuffix(payload, ";?") {
		return seq
	}
	num, arg, _ := strings.Cut(payload, ";")
	switch {
	case arg != "?":
	case num == "10" || num == "12": // foreground, cursor color
		r.reply("\x1b]" + num + ";" + r.fg + string(terminator))
	case num == "11": // background
		r.reply("\x1b]11;" + r.bg + string(terminator))
	}
	// Other queries (palette, clipboard, ...) are not answered, which is
	// what a terminal without the feature does.
	return nil
}

func (r *responder) reply(s string) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_, _ = io.WriteString(r.w, s)
}

// normalizeSGR rewrites SGR parameters into the plain semicolon form vt10x
// parses: colon sub-parameters (4:3, 38:2::r:g:b, 38:5:n) become their
// semicolon equivalents, and underline color (58/59), which vt10x would
// misread as other attributes, is dropped.
func normalizeSGR(params string) string {
	if params == "" {
		return ""
	}
	groups := strings.Split(params, ";")
	var out []string
	for i := 0; i < len(groups); i++ {
		g := groups[i]
		if strings.Contains(g, ":") {
			sub := strings.Split(g, ":")
			switch sub[0] {
			case "4": // underline style: 4:0 is off, 4:1..4:5 are styles
				if len(sub) > 1 && sub[1] == "0" {
					out = append(out, "24")
				} else {
					out = append(out, "4")
				}
			case "38", "48":
				out = append(out, colonColor(sub)...)
			}
			// 58:... (underline color) and unknown colon groups are dropped.
			continue
		}
		switch g {
		case "38", "48":
			// Semicolon extended color: keep it with its arguments.
			n := extendedColorArgs(groups[i+1:])
			out = append(out, groups[i:i+1+n]...)
			i += n
		case "58":
			i += extendedColorArgs(groups[i+1:])
		case "59":
		default:
			out = append(out, g)
		}
	}
	return strings.Join(out, ";")
}

// extendedColorArgs returns how many parameters follow 38/48/58 in the
// semicolon form (5;n or 2;r;g;b).
func extendedColorArgs(rest []string) int {
	if len(rest) == 0 {
		return 0
	}
	switch rest[0] {
	case "5":
		return min(2, len(rest))
	case "2":
		return min(4, len(rest))
	}
	return 0
}

// colonColor converts 38:5:n, 38:2:r:g:b and 38:2:cs:r:g:b to semicolon form.
func colonColor(sub []string) []string {
	if len(sub) >= 3 && sub[1] == "5" {
		return []string{sub[0], "5", sub[2]}
	}
	if len(sub) >= 5 && sub[1] == "2" {
		rgb := sub[2:5]
		if len(sub) >= 6 {
			rgb = sub[3:6] // with color space id
		}
		for k := range rgb {
			if rgb[k] == "" {
				rgb[k] = "0"
			}
		}
		return append([]string{sub[0], "2"}, rgb...)
	}
	return nil
}

// ParseColor converts "#rrggbb" to the rgb:rrrr/gggg/bbbb form used in OSC
// color replies.
func ParseColor(s string) (string, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 {
		return "", fmt.Errorf("invalid color %q (want #rrggbb)", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return "", fmt.Errorf("invalid color %q (want #rrggbb)", s)
	}
	r, g, b := (v>>16)&0xff, (v>>8)&0xff, v&0xff
	return fmt.Sprintf("rgb:%04x/%04x/%04x", r*0x101, g*0x101, b*0x101), nil
}
