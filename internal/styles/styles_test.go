package styles

import (
	"strings"
	"testing"

	"github.com/thinkbig1979/tui-goggles/internal/terminal"
)

const (
	dfg = terminal.DefaultFG
	dbg = terminal.DefaultBG
)

// row builds a row of cells from text, all with the given style.
func row(text string, fg, bg terminal.Color, attrs terminal.Attr) []terminal.Cell {
	var cells []terminal.Cell
	for _, r := range text {
		cells = append(cells, terminal.Cell{Char: r, FG: fg, BG: bg, Attrs: attrs})
	}
	return cells
}

func screen() [][]terminal.Cell {
	tabs := append(row(" Tab 1 ", dfg, dbg, terminal.AttrReverse|terminal.AttrBold),
		row(" Tab 2 ", 244, dbg, 0)...)
	tabs = append(tabs, row("   ", dfg, dbg, 0)...)
	body := append(row("sel", 0xff0000, 0x1e1e2e, terminal.AttrUnderline), row("ected plain   ", dfg, dbg, 0)...)
	blank := row(strings.Repeat(" ", 17), dfg, dbg, 0)
	return [][]terminal.Cell{tabs, body[:17], blank}
}

func TestSpans(t *testing.T) {
	spans := Spans(screen())
	if len(spans) != 3 {
		t.Fatalf("got %d spans; want 3: %+v", len(spans), spans)
	}
	s := spans[0]
	if s.Row != 0 || s.Col != 0 || s.Len != 7 || s.Text != " Tab 1 " || s.FG != "default" || s.BG != "default" ||
		strings.Join(s.Attrs, ",") != "bold,reverse" {
		t.Errorf("span 0 = %+v", s)
	}
	if s := spans[1]; s.Col != 7 || s.Text != " Tab 2 " || s.FG != "ansi:244" || len(s.Attrs) != 0 {
		t.Errorf("span 1 = %+v", s)
	}
	if s := spans[2]; s.Row != 1 || s.Text != "sel" || s.FG != "#ff0000" || s.BG != "#1e1e2e" || s.Attrs[0] != "underline" {
		t.Errorf("span 2 = %+v", s)
	}
	out := FormatSpans(spans[:1])
	if out != "row 0 col 0-6 \" Tab 1 \" bold reverse fg=default bg=default\n" {
		t.Errorf("FormatSpans = %q", out)
	}
}

func TestAssertions(t *testing.T) {
	cells := screen()
	pass := []string{
		`text="Tab 1" reverse bold`,
		`text="Tab 2" !reverse !bold fg=ansi:244`,
		`text="Tab 2" fg=244 bg=default`,
		`0,0 reverse`,
		`0,0,7 reverse bold`,
		`text=sel underline fg=#FF0000 bg=#1e1e2e`,
		`text=plain plain`,
		`14,0,3 plain`,
	}
	for _, spec := range pass {
		a, err := ParseAssertion(spec)
		if err != nil {
			t.Errorf("ParseAssertion(%q): %v", spec, err)
			continue
		}
		if err := a.Check(cells); err != nil {
			t.Errorf("%q: %v", spec, err)
		}
	}

	fail := map[string]string{
		`text="Tab 2" reverse`: `cell 8,0 'T': expected reverse, got fg=ansi:244 bg=default`,
		`text="Tab 1" !bold`:   `expected !bold`,
		`text=Missing bold`:    `text "Missing" not found`,
		`0,0,8 reverse`:        `cell 7,0`,
		`text=sel fg=#00ff00`:  `expected fg=#00ff00, got underline fg=#ff0000 bg=#1e1e2e`,
		`16,2,5 plain`:         `outside the screen`,
		`text="Tab 1" plain`:   `expected plain`,
	}
	for spec, want := range fail {
		a, err := ParseAssertion(spec)
		if err != nil {
			t.Errorf("ParseAssertion(%q): %v", spec, err)
			continue
		}
		err = a.Check(cells)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v; want it to contain %q", spec, err, want)
		}
	}
}

func TestParseAssertionErrors(t *testing.T) {
	for _, spec := range []string{
		"", "text=Tab", "1,2", "a,b bold", "1 bold", "1,2,0 bold", "1,2,3,4 bold",
		"text= bold", "1,2 shiny", "1,2 fg=red", "1,2 bg=ansi:300", `text="open bold`,
	} {
		if _, err := ParseAssertion(spec); err == nil {
			t.Errorf("ParseAssertion(%q) succeeded; want error", spec)
		}
	}
}

func TestColorName(t *testing.T) {
	tests := map[terminal.Color]string{
		dfg: "default", dbg: "default", 0: "ansi:0", 15: "ansi:15", 244: "ansi:244",
		0x1e1e2e: "#1e1e2e", 0xffffff: "#ffffff",
	}
	for c, want := range tests {
		if got := ColorName(c); got != want {
			t.Errorf("ColorName(%#x) = %q; want %q", uint32(c), got, want)
		}
	}
}
