package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/aliefe04/portico/internal/sshconfig"
	"github.com/aliefe04/portico/internal/sshconfigedit"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.filter.Width = msg.Width - 2
		if m.filter.Width < 0 {
			m.filter.Width = 0
		}
		m.resizeEditorFields()
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
		m.configPath = msg.result.Path
		m.hosts = append([]sshconfig.Host(nil), msg.result.Hosts...)
		m.applyFilter()
		m.previewDoc = nil
		m.preview = ""
		m.editorFields = nil
		m.editPath = ""
		m.editOriginalAlias = ""
		m.browseErr = nil
		m.editErr = nil
	case ConnectFinishedMsg:
		m.state = stateReady
		m.browseErr = msg.Err
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.state == stateLoading && (keyMsg.Type == tea.KeyEsc || keyMsg.Type == tea.KeyCtrlC) {
			return m, tea.Quit
		}
		switch m.state {
		case stateError:
			if keyMsg.Type == tea.KeyEsc || keyMsg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			return m, nil
		case stateReady:
			return m.updateReady(keyMsg)
		case stateEditing:
			return m.updateEditing(keyMsg)
		case statePreview:
			return m.updatePreview(keyMsg)
		case stateConfirmDelete:
			return m.updateDeleteConfirm(keyMsg)
		}
	}

	if m.state != stateReady {
		return m, nil
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

func (m Model) updateReady(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return m, tea.Quit
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
	case tea.KeyCtrlN:
		m.browseErr = nil
		return m.startCreate(), nil
	case tea.KeyCtrlE:
		if host, ok := m.currentHost(); ok {
			m.browseErr = nil
			return m.startEdit(host), nil
		}
		return m, nil
	case tea.KeyCtrlD:
		if host, ok := m.currentHost(); ok {
			m.browseErr = nil
			m.state = stateConfirmDelete
			m.editOriginalAlias = host.Alias
			m.editPath = host.FilePath()
			if m.editPath == "" {
				m.editPath = m.configPath
			}
			return m, nil
		}
		return m, nil
	case tea.KeyEnter:
		if host, ok := m.currentHost(); ok {
			m.browseErr = nil
			return m, m.deps.ConnectHost(host.Alias)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(keyMsg)
	m.browseErr = nil
	m.applyFilter()
	return m, cmd
}

func (m Model) updateEditing(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.editorFields) == 0 {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		m.state = stateReady
		m.editorFields = nil
		m.previewDoc = nil
		m.preview = ""
		m.editErr = nil
		return m, nil
	case tea.KeyCtrlS:
		return m.preparePreview()
	case tea.KeyDown:
		m.moveEditorFocus(1)
		return m, nil
	case tea.KeyUp:
		m.moveEditorFocus(-1)
		return m, nil
	case tea.KeyEnter:
		if m.editorIndex == len(m.editorFields)-1 {
			return m.preparePreview()
		}
		m.moveEditorFocus(1)
		return m, nil
	}

	field := m.editorFields[m.editorIndex]
	updatedField, cmd := field.Update(keyMsg)
	m.editorFields[m.editorIndex] = updatedField
	m.editErr = nil
	m.syncDraftFromEditor()
	return m, cmd
}

func (m Model) updatePreview(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEsc:
		m.state = stateEditing
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		return m.savePreview()
	case tea.KeyRunes:
		switch keyMsg.String() {
		case "s":
			return m.savePreview()
		case "e":
			m.state = stateEditing
			return m, nil
		}
	}

	return m, nil
}

func (m Model) updateDeleteConfirm(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEsc:
		m.state = stateReady
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		return m.deleteHost()
	case tea.KeyRunes:
		switch keyMsg.String() {
		case "y":
			return m.deleteHost()
		case "n":
			m.state = stateReady
			return m, nil
		}
	}

	return m, nil
}

func (m Model) preparePreview() (tea.Model, tea.Cmd) {
	m.syncDraftFromEditor()
	if strings.TrimSpace(m.draft.Alias) == "" {
		m.editErr = fmt.Errorf("alias is required")
		m.state = stateEditing
		return m, nil
	}
	if err := m.ensureDraftAliasUnique(); err != nil {
		m.editErr = err
		m.state = stateEditing
		return m, nil
	}

	doc, err := sshconfigedit.LoadDocument(m.editPath)
	if err != nil {
		m.editErr = err
		m.state = stateEditing
		return m, nil
	}

	switch m.editMode {
	case editModeCreate:
		if err := doc.CreateHost(m.draft); err != nil {
			m.editErr = err
			m.state = stateEditing
			return m, nil
		}
	case editModeUpdate:
		if err := doc.UpdateHost(m.editOriginalAlias, m.draft); err != nil {
			m.editErr = err
			m.state = stateEditing
			return m, nil
		}
	}

	m.previewDoc = doc
	m.preview = doc.Preview()
	m.state = statePreview
	return m, nil
}

func (m Model) savePreview() (tea.Model, tea.Cmd) {
	if m.previewDoc == nil {
		return m.preparePreview()
	}

	if err := m.previewDoc.Save(); err != nil {
		m.editErr = err
		m.state = statePreview
		return m, nil
	}

	return m.reloadHosts()
}

func (m Model) deleteHost() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.editPath) == "" {
		m.state = stateReady
		return m, nil
	}

	doc, err := sshconfigedit.LoadDocument(m.editPath)
	if err != nil {
		m.editErr = err
		m.state = stateConfirmDelete
		return m, nil
	}

	if err := doc.DeleteHost(m.editOriginalAlias); err != nil {
		m.editErr = err
		m.state = stateConfirmDelete
		return m, nil
	}

	if err := doc.Save(); err != nil {
		m.editErr = err
		m.state = stateConfirmDelete
		return m, nil
	}

	return m.reloadHosts()
}

func (m Model) reloadHosts() (tea.Model, tea.Cmd) {
	result, err := m.deps.LoadHosts()
	if err != nil {
		m.state = stateError
		m.err = err
		m.hosts = nil
		m.visible = nil
		m.selected = 0
		return m, nil
	}

	m.state = stateReady
	m.err = nil
	m.configPath = result.Path
	m.hosts = append([]sshconfig.Host(nil), result.Hosts...)
	m.visible = nil
	m.selected = 0
	m.previewDoc = nil
	m.preview = ""
	m.editErr = nil
	m.editorFields = nil
	m.editPath = ""
	m.editOriginalAlias = ""
	m.applyFilter()
	return m, nil
}

func (m Model) ensureDraftAliasUnique() error {
	alias := strings.TrimSpace(m.draft.Alias)
	for _, host := range m.hosts {
		if strings.TrimSpace(host.Alias) != alias {
			continue
		}
		if m.editMode == editModeUpdate && host.Alias == m.editOriginalAlias && host.FilePath() == m.editPath {
			continue
		}
		return fmt.Errorf("sshconfigedit: host %q already exists", alias)
	}
	return nil
}

func (m *Model) moveEditorFocus(delta int) {
	if len(m.editorFields) == 0 {
		return
	}

	m.editorFields[m.editorIndex].Blur()
	m.editorIndex = (m.editorIndex + delta + len(m.editorFields)) % len(m.editorFields)
	m.editorFields[m.editorIndex].Focus()
}

func (m *Model) syncDraftFromEditor() {
	if len(m.editorFields) == 0 {
		return
	}

	m.draft.Alias = strings.TrimSpace(m.editorFields[0].Value())
	m.draft.Hostname = strings.TrimSpace(m.editorFields[1].Value())
	m.draft.User = strings.TrimSpace(m.editorFields[2].Value())
	m.draft.Port = strings.TrimSpace(m.editorFields[3].Value())
	m.draft.ProxyJump = strings.TrimSpace(m.editorFields[4].Value())
	files := strings.Split(m.editorFields[5].Value(), ",")
	m.draft.IdentityFiles = m.draft.IdentityFiles[:0]
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed != "" {
			m.draft.IdentityFiles = append(m.draft.IdentityFiles, trimmed)
		}
	}
}

func (m *Model) resizeEditorFields() {
	if len(m.editorFields) == 0 {
		return
	}

	width := m.width - 8
	if width < 20 {
		width = 20
	}
	for i := range m.editorFields {
		m.editorFields[i].Width = width
	}
}
