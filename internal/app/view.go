package app

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/portico-dev/portico/internal/sshconfig"
	"github.com/portico-dev/portico/internal/ui"
)

const wideLayoutThreshold = 100

func (m Model) View() string {
	switch m.state {
	case stateLoading:
		return ui.Muted.Render("Loading SSH config...")
	case stateError:
		return ui.Error.Render(fmt.Sprintf("error loading SSH config: %s", sanitizeTerminalText(fmt.Sprint(m.err))))
	case stateEditing:
		return m.renderEditor()
	case statePreview:
		return m.renderPreview()
	case stateConfirmDelete:
		return m.renderDeleteConfirm()
	default:
		return m.renderBrowse()
	}
}

func (m Model) renderBrowse() string {
	parts := []string{
		ui.Title.Render("Portico"),
		ui.Muted.Render("Filter by alias or hostname"),
		m.filter.View(),
	}

	if len(m.visible) == 0 {
		parts = append(parts, ui.Muted.Render("No hosts match the current filter."))
	} else if m.useWideLayout() {
		parts = append(parts, m.renderSplitLayout())
	} else {
		parts = append(parts, m.renderHostList(), ui.Muted.Render("Selected host"), m.renderHostDetails(m.visible[m.selected]))
	}

	if m.browseErr != nil {
		parts = append(parts, ui.Error.Render(sanitizeTerminalText(m.browseErr.Error())))
	}

	parts = append(parts, ui.Muted.Render("enter: connect  ctrl+n: new  ctrl+e: edit  ctrl+d: delete  up/down: move  esc/ctrl+c: quit"))
	return strings.Join(parts, "\n\n")
}

func (m Model) renderEditor() string {
	labels := editorFieldPlaceholders()
	parts := []string{
		ui.Title.Render(map[editMode]string{editModeCreate: "Create host", editModeUpdate: "Edit host"}[m.editMode]),
		ui.Muted.Render("up/down: move  ctrl+s/enter: preview  esc: cancel"),
	}
	if m.editErr != nil {
		parts = append(parts, ui.Error.Render(sanitizeTerminalText(m.editErr.Error())))
	}

	for i, field := range m.editorFields {
		marker := " "
		if i == m.editorIndex {
			marker = ">"
		}
		parts = append(parts, fmt.Sprintf("%s %s: %s", marker, labels[i], sanitizeTerminalText(field.Value())))
	}

	return strings.Join(parts, "\n\n")
}

func (m Model) renderPreview() string {
	parts := []string{
		ui.Title.Render("Save preview"),
		ui.Muted.Render("s/enter: save  e: edit  esc: back"),
	}
	if m.editErr != nil {
		parts = append(parts, ui.Error.Render(sanitizeTerminalText(m.editErr.Error())))
	}
	parts = append(parts,
		ui.Panel.Render(sanitizeMultilineText(m.preview)),
	)
	return strings.Join(parts, "\n\n")
}

func (m Model) renderDeleteConfirm() string {
	parts := []string{
		ui.Title.Render("Delete host"),
		ui.Error.Render(fmt.Sprintf("Delete %s?", sanitizeTerminalText(m.editOriginalAlias))),
		ui.Muted.Render("y/enter: delete  n/esc: cancel"),
	}
	if m.editErr != nil {
		parts = append(parts, ui.Error.Render(sanitizeTerminalText(m.editErr.Error())))
	}
	return strings.Join(parts, "\n\n")
}

func (m Model) useWideLayout() bool {
	return m.width >= wideLayoutThreshold && len(m.visible) > 0
}

func (m Model) renderSplitLayout() string {
	leftWidth := m.width / 3
	if leftWidth < 28 {
		leftWidth = 28
	}
	if leftWidth > 40 {
		leftWidth = 40
	}
	rightWidth := m.width - leftWidth - 4

	left := ui.Panel.Width(leftWidth).Render(strings.Join([]string{
		ui.PanelTitle.Render("Hosts"),
		m.renderHostList(),
	}, "\n\n"))
	right := ui.Panel.Width(rightWidth).Render(strings.Join([]string{
		ui.PanelTitle.Render("Details"),
		m.renderHostDetails(m.visible[m.selected]),
	}, "\n\n"))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderHostList() string {
	lines := make([]string, 0, len(m.visible))
	for i, host := range m.visible {
		marker := " "
		if i == m.selected {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s", marker, sanitizeTerminalText(host.Alias)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHostDetails(host sshconfig.Host) string {
	identityFiles := "-"
	if len(host.IdentityFiles) > 0 {
		sanitized := make([]string, 0, len(host.IdentityFiles))
		for _, path := range host.IdentityFiles {
			sanitized = append(sanitized, sanitizeTerminalText(path))
		}
		identityFiles = strings.Join(sanitized, ", ")
	}

	proxyJump := sanitizeTerminalText(host.ProxyJump)
	if proxyJump == "" {
		proxyJump = "-"
	}

	user := sanitizeTerminalText(host.User)
	if user == "" {
		user = "-"
	}

	port := sanitizeTerminalText(host.Port)
	if port == "" {
		port = "-"
	}

	return strings.Join([]string{
		fmt.Sprintf("Alias: %s", sanitizeTerminalText(host.Alias)),
		fmt.Sprintf("Hostname: %s", sanitizeTerminalText(host.Hostname)),
		fmt.Sprintf("User: %s", user),
		fmt.Sprintf("Port: %s", port),
		fmt.Sprintf("ProxyJump: %s", proxyJump),
		fmt.Sprintf("IdentityFiles: %s", identityFiles),
	}, "\n")
}

func sanitizeTerminalText(value string) string {
	if value == "" {
		return ""
	}

	b := strings.Builder{}
	b.Grow(len(value))
	lastWasSpace := false

	for _, r := range value {
		switch {
		case r == '\x1b' || unicode.Is(unicode.Cf, r):
			continue
		case r == '\n' || r == '\r' || r == '\t' || r == '\a' || r == '\f' || r == '\v' || r == '\x00':
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		case unicode.IsControl(r):
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		default:
			b.WriteRune(r)
			lastWasSpace = false
		}
	}

	return strings.TrimSpace(b.String())
}

func sanitizeMultilineText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = sanitizeTerminalText(line)
	}
	return strings.Join(lines, "\n")
}
