package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/portico-dev/portico/internal/app"
	"github.com/portico-dev/portico/internal/platform"
	"github.com/portico-dev/portico/internal/sshconfig"
	"github.com/portico-dev/portico/internal/version"
)

func main() {
	home, err := platform.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "portico: resolve home directory: %v\n", err)
		os.Exit(1)
	}

	configPath := platform.DefaultSSHConfigPath(home)
	m := app.New(app.Dependencies{
		Version: version.Summary(),
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
	})
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "portico: %v\n", err)
		os.Exit(1)
	}
}
