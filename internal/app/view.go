package app

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/portico-dev/portico/internal/sshconfig"
	"github.com/portico-dev/portico/internal/ui"
)

func (m Model) View() string {
	switch m.state {
	case stateLoading:
		return ui.Muted.Render("Loading SSH config...")
	case stateError:
		return ui.Error.Render(fmt.Sprintf("error loading SSH config: %s", sanitizeTerminalText(fmt.Sprint(m.err))))
	default:
		parts := []string{
			ui.Title.Render("Portico"),
			ui.Muted.Render("Filter by alias or hostname"),
			m.filter.View(),
		}

		if len(m.visible) == 0 {
			parts = append(parts, ui.Muted.Render("No hosts match the current filter."))
		} else {
			parts = append(parts, m.renderHostList(), ui.Muted.Render("Selected host"), m.renderHostDetails(m.visible[m.selected]))
		}

		parts = append(parts, ui.Muted.Render("up/down: move  esc/ctrl+c: quit"))
		return strings.Join(parts, "\n\n")
	}
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
