package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aliefe04/portico/internal/app"
	"github.com/aliefe04/portico/internal/platform"
	"github.com/aliefe04/portico/internal/sshconfig"
	"github.com/aliefe04/portico/internal/version"
)

func main() {
	home, err := platform.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "portico: resolve home directory: %s\n", sanitizeTerminalText(fmt.Sprint(err)))
		os.Exit(1)
	}

	configPath := platform.DefaultSSHConfigPath(home)
	m := app.New(app.Dependencies{
		Version: version.Summary(),
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
		ConnectHost: func(alias string) tea.Cmd {
			cmd := exec.Command("ssh", alias)
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return app.ConnectFinishedMsg{Alias: alias, Err: err}
			})
		},
	})
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "portico: %s\n", sanitizeTerminalText(fmt.Sprint(err)))
		os.Exit(1)
	}
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
