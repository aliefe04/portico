package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/portico-dev/portico/internal/sshconfig"
	"github.com/portico-dev/portico/internal/sshconfigedit"
)

type Dependencies struct {
	Version     string
	LoadHosts   func() (sshconfig.Result, error)
	ConnectHost func(alias string) tea.Cmd
}

type modelState int

const (
	stateLoading modelState = iota
	stateReady
	stateEditing
	statePreview
	stateConfirmDelete
	stateError
)

type editMode int

const (
	editModeCreate editMode = iota
	editModeUpdate
)

type loadHostsMsg struct {
	result sshconfig.Result
	err    error
}

type ConnectFinishedMsg struct {
	Alias string
	Err   error
}

type Model struct {
	deps              Dependencies
	state             modelState
	filter            textinput.Model
	width             int
	configPath        string
	hosts             []sshconfig.Host
	visible           []sshconfig.Host
	selected          int
	draft             sshconfigedit.HostDraft
	editMode          editMode
	editPath          string
	editOriginalAlias string
	editorFields      []textinput.Model
	editorIndex       int
	previewDoc        *sshconfigedit.Document
	preview           string
	browseErr         error
	editErr           error
	err               error
}

func New(deps Dependencies) Model {
	if deps.LoadHosts == nil {
		deps.LoadHosts = func() (sshconfig.Result, error) {
			return sshconfig.Result{}, nil
		}
	}
	if deps.ConnectHost == nil {
		deps.ConnectHost = func(string) tea.Cmd {
			return nil
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

func editorFieldPlaceholders() []string {
	return []string{"Alias", "Hostname", "User", "Port", "ProxyJump", "Identity files (comma separated)"}
}

func newEditorFields(draft sshconfigedit.HostDraft) []textinput.Model {
	values := []string{draft.Alias, draft.Hostname, draft.User, draft.Port, draft.ProxyJump, strings.Join(draft.IdentityFiles, ", ")}
	placeholders := editorFieldPlaceholders()
	fields := make([]textinput.Model, len(values))
	for i := range values {
		field := textinput.New()
		field.Placeholder = placeholders[i]
		field.SetValue(values[i])
		field.Prompt = ""
		fields[i] = field
	}
	if len(fields) > 0 {
		fields[0].Focus()
	}
	return fields
}

func (m Model) withEditorDraft(mode editMode, path, originalAlias string, draft sshconfigedit.HostDraft) Model {
	m.state = stateEditing
	m.editMode = mode
	m.editPath = path
	m.editOriginalAlias = originalAlias
	m.draft = draft
	m.editorFields = newEditorFields(draft)
	m.editorIndex = 0
	m.previewDoc = nil
	m.preview = ""
	m.editErr = nil
	return m
}

func draftFromHost(host sshconfig.Host) sshconfigedit.HostDraft {
	return sshconfigedit.HostDraft{
		Alias:         host.Alias,
		Hostname:      host.Hostname,
		User:          host.User,
		Port:          host.Port,
		ProxyJump:     host.ProxyJump,
		IdentityFiles: append([]string(nil), host.IdentityFiles...),
	}
}

func (m Model) currentHost() (sshconfig.Host, bool) {
	if len(m.visible) == 0 || m.selected < 0 || m.selected >= len(m.visible) {
		return sshconfig.Host{}, false
	}

	return m.visible[m.selected], true
}

func (m Model) startCreate() Model {
	return m.withEditorDraft(editModeCreate, m.configPath, "", sshconfigedit.HostDraft{})
}

func (m Model) startEdit(host sshconfig.Host) Model {
	path := host.FilePath()
	if path == "" {
		path = m.configPath
	}
	return m.withEditorDraft(editModeUpdate, path, host.Alias, draftFromHost(host))
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		result, err := m.deps.LoadHosts()
		return loadHostsMsg{result: result, err: err}
	}
}
