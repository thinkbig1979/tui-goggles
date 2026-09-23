// Package styles summarizes styled screen regions as spans and checks
// style assertions against them.
package styles

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/your-username/tui-goggles/internal/input"
	"github.com/your-username/tui-goggles/internal/terminal"
)

// Span is a run of cells on one row that share a non-default style.
type Span struct {
	Row   int      `json:"row"`
	Col   int      `json:"col"`
	Len   int      `json:"len"`
	Text  string   `json:"text"`
	FG    string   `json:"fg"`
	BG    string   `json:"bg"`
	Attrs []string `json:"attrs,omitempty"`
}

// Style is the style of one cell in comparable, printable form.
type Style struct {
	FG, BG string
	Attrs  terminal.Attr
}

// Plain reports whether the style is the terminal default.
func (s Style) Plain() bool {
	return s.FG == "default" && s.BG == "default" && s.Attrs == 0
}

// String formats the style like "bold reverse fg=#ff0000 bg=default".
func (s Style) String() string {
	parts := AttrNames(s.Attrs)
	parts = append(parts, "fg="+s.FG, "bg="+s.BG)
	return strings.Join(parts, " ")
}

// CellStyle returns the style of a cell.
func CellStyle(c terminal.Cell) Style {
	return Style{FG: ColorName(c.FG), BG: ColorName(c.BG), Attrs: c.Attrs}
}

// ColorName formats a color as "default", "ansi:N" (palette index 0-255)
// or "#rrggbb".
func ColorName(c terminal.Color) string {
	switch {
	case c == terminal.DefaultFG || c == terminal.DefaultBG:
		return "default"
	case c < 256:
		return "ansi:" + strconv.Itoa(int(c))
	case c < 1<<24:
		return fmt.Sprintf("#%06x", uint32(c))
	}
	return "default"
}

var attrNames = []struct {
	attr terminal.Attr
	name string
}{
	{terminal.AttrBold, "bold"},
	{terminal.AttrItalic, "italic"},
	{terminal.AttrUnderline, "underline"},
	{terminal.AttrBlink, "blink"},
	{terminal.AttrReverse, "reverse"},
}

// AttrNames lists the names of the attributes set in a.
func AttrNames(a terminal.Attr) []string {
	var names []string
	for _, n := range attrNames {
		if a&n.attr != 0 {
			names = append(names, n.name)
		}
	}
	return names
}

// Spans returns the styled runs on screen, row by row, omitting runs in
// the default style.
func Spans(cells [][]terminal.Cell) []Span {
	var spans []Span
	for y, row := range cells {
		for x := 0; x < len(row); {
			st := CellStyle(row[x])
			end := x + 1
			for end < len(row) && CellStyle(row[end]) == st {
				end++
			}
			if !st.Plain() {
				var text strings.Builder
				for _, c := range row[x:end] {
					text.WriteRune(c.Char)
				}
				spans = append(spans, Span{
					Row: y, Col: x, Len: end - x, Text: text.String(),
					FG: st.FG, BG: st.BG, Attrs: AttrNames(st.Attrs),
				})
			}
			x = end
		}
	}
	return spans
}

// FormatSpans renders spans as text, one per line:
//
//	row 0 col 0-6 " Tab 1 " bold reverse fg=default bg=default
func FormatSpans(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		fmt.Fprintf(&b, "row %d col %d-%d %q", s.Row, s.Col, s.Col+s.Len-1, s.Text)
		for _, a := range s.Attrs {
			b.WriteString(" " + a)
		}
		fmt.Fprintf(&b, " fg=%s bg=%s\n", s.FG, s.BG)
	}
	return b.String()
}

// Assertion is a parsed style assertion: a cell selector and the
// expectations every selected cell must meet.
type Assertion struct {
	Spec string

	// Selector: either a text search or a cell range on one row.
	Text     string
	X, Y, N  int
	expected []expectation
}

type expectation struct {
	raw    string
	attr   terminal.Attr // attribute check if non-zero
	negate bool
	fg, bg string // color checks if non-empty
	plain  bool
}

// ParseAssertion parses a style assertion:
//
//	SELECTOR EXPECTATION...
//
// SELECTOR is X,Y (one cell), X,Y,LEN (LEN cells from X on row Y) or
// text=STRING (the first occurrence of STRING on screen, quoted if it has
// spaces: text="Tab 2"). Coordinates are 0-based. EXPECTATIONs are bold,
// italic, underline, blink, reverse (prefix ! to require absence),
// fg=COLOR, bg=COLOR and plain (default colors, no attributes). COLOR is
// default, ansi:N, N or #rrggbb.
func ParseAssertion(spec string) (Assertion, error) {
	a := Assertion{Spec: spec}
	tokens, err := input.Tokenize(spec)
	if err != nil {
		return a, err
	}
	if len(tokens) < 2 {
		return a, fmt.Errorf("style assertion %q needs a selector and at least one expectation", spec)
	}

	sel := tokens[0]
	if text, ok := strings.CutPrefix(sel, "text="); ok {
		if text == "" {
			return a, fmt.Errorf("empty text selector in %q", spec)
		}
		a.Text = text
	} else {
		parts := strings.Split(sel, ",")
		nums := make([]int, len(parts))
		for i, p := range parts {
			if nums[i], err = strconv.Atoi(p); err != nil || nums[i] < 0 {
				return a, fmt.Errorf("invalid selector %q (want X,Y  X,Y,LEN  or text=STRING)", sel)
			}
		}
		switch len(nums) {
		case 2:
			a.X, a.Y, a.N = nums[0], nums[1], 1
		case 3:
			a.X, a.Y, a.N = nums[0], nums[1], nums[2]
		default:
			return a, fmt.Errorf("invalid selector %q (want X,Y  X,Y,LEN  or text=STRING)", sel)
		}
		if a.N < 1 {
			return a, fmt.Errorf("invalid selector %q: LEN must be at least 1", sel)
		}
	}

	for _, tok := range tokens[1:] {
		e, err := parseExpectation(tok)
		if err != nil {
			return a, err
		}
		a.expected = append(a.expected, e)
	}
	return a, nil
}

func parseExpectation(tok string) (expectation, error) {
	e := expectation{raw: tok}
	if v, ok := strings.CutPrefix(tok, "fg="); ok {
		c, err := normalizeColor(v)
		e.fg = c
		return e, err
	}
	if v, ok := strings.CutPrefix(tok, "bg="); ok {
		c, err := normalizeColor(v)
		e.bg = c
		return e, err
	}
	if tok == "plain" {
		e.plain = true
		return e, nil
	}
	name := tok
	if strings.HasPrefix(name, "!") {
		e.negate = true
		name = name[1:]
	}
	for _, n := range attrNames {
		if n.name == name {
			e.attr = n.attr
			return e, nil
		}
	}
	return e, fmt.Errorf("unknown style expectation %q (want bold, italic, underline, blink, reverse, !ATTR, fg=, bg= or plain)", tok)
}

func normalizeColor(v string) (string, error) {
	v = strings.ToLower(v)
	switch {
	case v == "default":
		return v, nil
	case strings.HasPrefix(v, "#") && len(v) == 7:
		if _, err := strconv.ParseUint(v[1:], 16, 32); err == nil {
			return v, nil
		}
	default:
		n, err := strconv.Atoi(strings.TrimPrefix(v, "ansi:"))
		if err == nil && n >= 0 && n < 256 {
			return "ansi:" + strconv.Itoa(n), nil
		}
	}
	return "", fmt.Errorf("invalid color %q (want default, ansi:N, N or #rrggbb)", v)
}

// Check evaluates the assertion against the screen and returns an error
// describing the first mismatch.
func (a Assertion) Check(cells [][]terminal.Cell) error {
	x, y, n := a.X, a.Y, a.N
	if a.Text != "" {
		var found bool
		x, y, found = findText(cells, a.Text)
		if !found {
			return fmt.Errorf("text %q not found on screen", a.Text)
		}
		n = len([]rune(a.Text))
	}
	if y >= len(cells) || x+n > len(cells[y]) {
		return fmt.Errorf("cells %d,%d+%d are outside the screen", x, y, n)
	}

	for i := x; i < x+n; i++ {
		st := CellStyle(cells[y][i])
		for _, e := range a.expected {
			if !e.matches(st) {
				return fmt.Errorf("cell %d,%d %q: expected %s, got %s", i, y, cells[y][i].Char, e.raw, st)
			}
		}
	}
	return nil
}

func (e expectation) matches(st Style) bool {
	switch {
	case e.plain:
		return st.Plain()
	case e.fg != "":
		return st.FG == e.fg
	case e.bg != "":
		return st.BG == e.bg
	}
	has := st.Attrs&e.attr != 0
	return has != e.negate
}

// findText returns the cell position of the first occurrence of text,
// scanning rows top to bottom.
func findText(cells [][]terminal.Cell, text string) (x, y int, found bool) {
	want := []rune(text)
	for y, row := range cells {
		for x := 0; x+len(want) <= len(row); x++ {
			match := true
			for i, r := range want {
				if row[x+i].Char != r {
					match = false
					break
				}
			}
			if match {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}
