package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aliefe04/portico/internal/app"
	"github.com/aliefe04/portico/internal/platform"
	"github.com/aliefe04/portico/internal/sshconfig"
	"github.com/aliefe04/portico/internal/update"
	"github.com/aliefe04/portico/internal/version"
)

func main() {
	if handled, code := runCLICommand(); handled {
		os.Exit(code)
	}

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

func runCLICommand() (bool, int) {
	if len(os.Args) < 2 {
		return false, 0
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println(version.Version)
		return true, 0
	case "self-update":
		if err := runSelfUpdate(); err != nil {
			fmt.Fprintf(os.Stderr, "portico: %s\n", sanitizeTerminalText(fmt.Sprint(err)))
			return true, 1
		}
		return true, 0
	default:
		return false, 0
	}
}

func runSelfUpdate() error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}

	result, err := (update.Updater{}).SelfUpdate(context.Background(), version.Version, executablePath, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	if !result.Updated {
		fmt.Printf("Portico %s is already up to date.\n", result.CurrentVersion)
		return nil
	}

	fmt.Printf("Updated Portico from %s to %s.\n", result.CurrentVersion, result.LatestVersion)
	return nil
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
