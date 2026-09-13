package tui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/sadisticbrew/meridian/internal/views"
)

const (
	subjectDays     = 14
	dsaDays         = 14
	refreshInterval = 30 * time.Second
	scrollPageMin   = 10
)

var tabTitles = []string{"Today", "Week", "Subject", "DSA"}

var (
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	gapStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// tickMsg drives the 30s auto-refresh; tests send it directly.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the read-only dashboard. NewModel renders the first tab so the
// program (and headless tests) start with content populated.
type Model struct {
	st    *store.Store
	nowFn func() time.Time

	Tab    int
	lines  []string
	offset int
	height int

	err      error
	quitting bool
}

func NewModel(st *store.Store, nowFn func() time.Time) *Model {
	if nowFn == nil {
		nowFn = time.Now
	}
	m := &Model{st: st, nowFn: nowFn}
	m.refresh()
	return m
}

func (m *Model) Init() tea.Cmd { return tickCmd() }

func (m *Model) content() string { return strings.Join(m.lines, "\n") }

func (m *Model) refresh() {
	var buf bytes.Buffer
	m.err = m.render(&buf)
	if m.err != nil {
		m.lines = strings.Split(m.err.Error(), "\n")
	} else if text := strings.TrimRight(buf.String(), "\n"); text != "" {
		m.lines = strings.Split(text, "\n")
	} else {
		m.lines = nil
	}
	m.clampOffset()
}

func (m *Model) render(w io.Writer) error {
	now := m.nowFn()
	switch m.Tab {
	case 0:
		return views.Today(w, now, m.st)
	case 1:
		return views.Week(w, now, m.st, false)
	case 2:
		return m.renderSubjects(w, now)
	case 3:
		return views.DSA(w, now, m.st, dsaDays)
	}
	return fmt.Errorf("unknown tab %d", m.Tab)
}

// renderSubjects stacks every active non-pattern subject's 14-day view,
// separated by a blank line.
func (m *Model) renderSubjects(w io.Writer, now time.Time) error {
	active, err := m.st.SubjectsActive()
	if err != nil {
		return err
	}
	first := true
	for _, t := range active {
		if t.Kind == "pattern" {
			continue
		}
		if !first {
			fmt.Fprintln(w)
		}
		if err := views.Subject(w, now, m.st, t.ID, subjectDays); err != nil {
			return err
		}
		first = false
	}
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 3
		if m.height < 0 {
			m.height = 0
		}
		m.clampOffset()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "1", "2", "3", "4":
			m.switchTab(int(msg.String()[0] - '1'))
		case "h":
			m.switchTab((m.Tab + len(tabTitles) - 1) % len(tabTitles))
		case "l":
			m.switchTab((m.Tab + 1) % len(tabTitles))
		case "r":
			m.refresh()
		case "up":
			m.scroll(-1)
		case "down":
			m.scroll(1)
		case "pgup":
			m.scroll(-m.page())
		case "pgdown":
			m.scroll(m.page())
		}
	case tickMsg:
		m.refresh()
		return m, tickCmd()
	}
	return m, nil
}

func (m *Model) switchTab(tab int) {
	m.Tab = tab
	m.offset = 0
	m.refresh()
}

func (m *Model) scroll(delta int) {
	m.offset += delta
	m.clampOffset()
}

func (m *Model) clampOffset() {
	max := len(m.lines) - m.height
	if m.height <= 0 || max < 0 {
		max = 0
	}
	if m.offset > max {
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) page() int {
	if m.height > 0 {
		return m.height
	}
	return scrollPageMin
}

func (m *Model) View() string {
	var b strings.Builder
	for i, title := range tabTitles {
		if i > 0 {
			b.WriteString("  ")
		}
		label := fmt.Sprintf("%d %s", i+1, title)
		if i == m.Tab {
			b.WriteString(activeTabStyle.Render(label))
		} else {
			b.WriteString(inactiveTabStyle.Render(label))
		}
	}
	b.WriteString("\n\n")
	lines := colored(m.lines)
	for _, line := range m.visible(lines) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(footerStyle.Render("1-4 tabs · h/l prev/next · r refresh · up/down scroll · q quit"))
	return b.String()
}

func (m *Model) visible(lines []string) []string {
	if m.height <= 0 || len(lines) <= m.height {
		return lines
	}
	off := m.offset
	if off > len(lines)-m.height {
		off = len(lines) - m.height
	}
	if off < 0 {
		off = 0
	}
	return lines[off : off+m.height]
}

// colored tints the captured plain-text Gaps section (the "Gaps" line and the
// ⚠ lines up to the next blank); views themselves stay plain.
func colored(lines []string) []string {
	out := make([]string, len(lines))
	inGaps := false
	for i, line := range lines {
		switch {
		case line == "Gaps":
			inGaps = true
			out[i] = gapStyle.Render(line)
		case inGaps && strings.HasPrefix(strings.TrimSpace(line), "⚠"):
			out[i] = gapStyle.Render(line)
		case inGaps && strings.TrimSpace(line) == "":
			inGaps = false
			out[i] = line
		default:
			out[i] = line
		}
	}
	return out
}
