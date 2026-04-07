package app

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/portico-dev/portico/internal/sshconfig"
)

type Dependencies struct {
	Version   string
	LoadHosts func() (sshconfig.Result, error)
}

type modelState int

const (
	stateLoading modelState = iota
	stateReady
	stateError
)

type loadHostsMsg struct {
	result sshconfig.Result
	err    error
}

type Model struct {
	deps     Dependencies
	state    modelState
	filter   textinput.Model
	width    int
	hosts    []sshconfig.Host
	visible  []sshconfig.Host
	selected int
	err      error
}

func New(deps Dependencies) Model {
	if deps.LoadHosts == nil {
		deps.LoadHosts = func() (sshconfig.Result, error) {
			return sshconfig.Result{}, nil
		}
	}

	filter := textinput.New()
	filter.Placeholder = "Filter hosts"
	filter.Focus()

	return Model{
		deps:   deps,
		state:  stateLoading,
		filter: filter,
	}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		result, err := m.deps.LoadHosts()
		return loadHostsMsg{result: result, err: err}
	}
}
