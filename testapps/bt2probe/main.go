// bt2probe is a small Bubble Tea v2 program used to demonstrate and test
// tui-goggles against real v2 input parsing. It runs in the alt screen with
// cell-motion mouse tracking and logs every input event it receives.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const maxLog = 12

var tabs = []string{"Tab 1", "Tab 2", "Tab 3"}

type model struct {
	width, height int
	active        int
	bg            string
	version       string
	modes         []string
	log           []string
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestTerminalVersion)
}

func (m *model) add(s string) {
	m.log = append(m.log, s)
	if len(m.log) > maxLog {
		m.log = m.log[len(m.log)-maxLog:]
	}
}

// tabAt returns the tab index under column x on the tab bar, or -1.
func tabAt(x int) int {
	col := 0
	for i, t := range tabs {
		w := len(t) + 2
		if x >= col && x < col+w {
			return i
		}
		col += w
	}
	return -1
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.add(fmt.Sprintf("resize: %dx%d", msg.Width, msg.Height))
	case tea.BackgroundColorMsg:
		m.bg = msg.String()
	case tea.TerminalVersionMsg:
		m.version = msg.Name
	case tea.ModeReportMsg:
		m.modes = append(m.modes, fmt.Sprintf("%d=%d", msg.Mode, msg.Value))
	case tea.KeyPressMsg:
		k := msg.String()
		m.add("key: " + k)
		switch k {
		case "ctrl+c":
			return m, tea.Quit
		case "alt+right", "ctrl+pgdown":
			m.active = (m.active + 1) % len(tabs)
		case "alt+left", "ctrl+pgup":
			m.active = (m.active + len(tabs) - 1) % len(tabs)
		}
	case tea.PasteMsg:
		m.add(fmt.Sprintf("paste: %q", msg.Content))
	case tea.MouseClickMsg:
		m.add(fmt.Sprintf("click: %s @ %d,%d", msg, msg.X, msg.Y))
		if msg.Y == 0 {
			if i := tabAt(msg.X); i >= 0 {
				m.active = i
			}
		}
	case tea.MouseReleaseMsg:
		m.add(fmt.Sprintf("release: %s @ %d,%d", msg, msg.X, msg.Y))
	case tea.MouseMotionMsg:
		m.add(fmt.Sprintf("motion: %s @ %d,%d", msg, msg.X, msg.Y))
	case tea.MouseWheelMsg:
		m.add(fmt.Sprintf("wheel: %s @ %d,%d", msg, msg.X, msg.Y))
	}
	return m, nil
}

var (
	activeTab   = lipgloss.NewStyle().Reverse(true).Bold(true)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	label       = lipgloss.NewStyle().Underline(true)
)

func (m model) View() tea.View {
	var b strings.Builder
	for i, t := range tabs {
		if i == m.active {
			b.WriteString(activeTab.Render(" " + t + " "))
		} else {
			b.WriteString(inactiveTab.Render(" " + t + " "))
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s %dx%d bg=%s ver=%s modes=%s\n", label.Render("term"),
		m.width, m.height, m.bg, m.version, strings.Join(m.modes, ","))
	b.WriteString(strings.Repeat("-", 20) + "\n")
	b.WriteString(strings.Join(m.log, "\n"))

	v := tea.NewView(b.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func main() {
	if _, err := tea.NewProgram(model{}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
