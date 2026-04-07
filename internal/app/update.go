package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/portico-dev/portico/internal/sshconfig"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.filter.Width = msg.Width - 2
		if m.filter.Width < 0 {
			m.filter.Width = 0
		}
		return m, nil
	case loadHostsMsg:
		if msg.err != nil {
			m.state = stateError
			m.err = msg.err
			m.hosts = nil
			m.visible = nil
			m.selected = 0
			return m, nil
		}

		m.state = stateReady
		m.err = nil
		m.hosts = append([]sshconfig.Host(nil), msg.result.Hosts...)
		m.applyFilter()
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		}
	}

	if m.state != stateReady {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyDown:
			if m.selected < len(m.visible)-1 {
				m.selected++
			}
			return m, nil
		case tea.KeyUp:
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.visible = m.visible[:0]

	for _, host := range m.hosts {
		if query == "" || strings.Contains(strings.ToLower(host.Alias), query) || strings.Contains(strings.ToLower(host.Hostname), query) {
			m.visible = append(m.visible, host)
		}
	}

	if len(m.visible) == 0 {
		m.selected = 0
		return
	}

	if m.selected >= len(m.visible) {
		m.selected = len(m.visible) - 1
	}
}
