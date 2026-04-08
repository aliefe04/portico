package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/portico-dev/portico/internal/sshconfig"
)

func TestModelInitLoadsHosts(t *testing.T) {
	loadResult := sshconfig.Result{
		Hosts: []sshconfig.Host{{Alias: "web"}},
	}

	called := false
	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			called = true
			return loadResult, nil
		},
	})

	if m.filter.Placeholder != "Filter hosts" {
		t.Fatalf("filter.Placeholder = %q, want %q", m.filter.Placeholder, "Filter hosts")
	}

	if !m.filter.Focused() {
		t.Fatal("filter should be focused")
	}

	if got := m.View(); !strings.Contains(got, "Loading SSH config...") {
		t.Fatalf("View() = %q, want loading text", got)
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil command")
	}

	msg := cmd()
	loadMsg, ok := msg.(loadHostsMsg)
	if !ok {
		t.Fatalf("Init() message type = %T, want loadHostsMsg", msg)
	}

	if !called {
		t.Fatal("LoadHosts was not called")
	}

	if loadMsg.err != nil {
		t.Fatalf("load message err = %v, want nil", loadMsg.err)
	}

	updated, _ := m.Update(loadMsg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() model type = %T, want Model", updated)
	}

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if len(next.visible) != 1 || next.visible[0].Alias != "web" {
		t.Fatalf("visible = %#v, want loaded hosts", next.visible)
	}

	loadResult.Hosts[0].Alias = "changed"
	if next.visible[0].Alias != "web" {
		t.Fatalf("visible aliases source slice: %#v", next.visible)
	}

	if got := next.View(); !strings.Contains(got, "Portico") {
		t.Fatalf("View() = %q, want Portico title", got)
	}
	if strings.Contains(next.View(), "Loading SSH config...") {
		t.Fatalf("View() = %q, should not show loading text after success", next.View())
	}

	_ = tea.WindowSizeMsg{}
	_ = loadResult
}

func TestModelInitUsesNoOpLoaderWhenNil(t *testing.T) {
	m := New(Dependencies{Version: "dev"})

	msg := m.Init()()
	loadMsg, ok := msg.(loadHostsMsg)
	if !ok {
		t.Fatalf("Init() message type = %T, want loadHostsMsg", msg)
	}

	if loadMsg.err != nil {
		t.Fatalf("load message err = %v, want nil", loadMsg.err)
	}

	updated, _ := m.Update(loadMsg)
	next := updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if len(next.visible) != 0 {
		t.Fatalf("visible = %#v, want empty slice", next.visible)
	}
}

func TestModelIgnoresHiddenInputOutsideReadyState(t *testing.T) {
	m := New(Dependencies{Version: "dev"})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	next := updated.(Model)

	if cmd != nil {
		t.Fatal("Update() returned unexpected command while loading")
	}

	if next.filter.Value() != "" {
		t.Fatalf("filter.Value() = %q, want empty string while loading", next.filter.Value())
	}

	if next.state != stateLoading {
		t.Fatalf("state = %v, want %v", next.state, stateLoading)
	}
}

func TestModelEscQuitsWhileLoading(t *testing.T) {
	m := New(Dependencies{Version: "dev"})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(Model)

	if next.state != stateLoading {
		t.Fatalf("state = %v, want %v", next.state, stateLoading)
	}

	if cmd == nil {
		t.Fatal("Update() returned nil command for esc while loading")
	}

	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("esc command result = %#v, want %#v", msg, tea.Quit())
	}
}

func TestModelShowsLoaderError(t *testing.T) {
	boom := errors.New("boom")
	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Result{}, boom
		},
	})

	msg := m.Init()()
	loadMsg, ok := msg.(loadHostsMsg)
	if !ok {
		t.Fatalf("Init() message type = %T, want loadHostsMsg", msg)
	}

	updated, _ := m.Update(loadMsg)
	next := updated.(Model)

	if next.state != stateError {
		t.Fatalf("state = %v, want %v", next.state, stateError)
	}

	if !errors.Is(next.err, boom) {
		t.Fatalf("err = %v, want %v", next.err, boom)
	}

	view := next.View()
	if !strings.Contains(view, "boom") {
		t.Fatalf("View() = %q, want loader error", view)
	}
	if !strings.Contains(view, "error") {
		t.Fatalf("View() = %q, want clear error text", view)
	}
	if strings.Contains(view, "Portico") {
		t.Fatalf("View() = %q, should not show title in error state", view)
	}
}

func TestModelCtrlCQuitsWhileShowingError(t *testing.T) {
	m := New(Dependencies{Version: "dev"})
	m.state = stateError
	m.err = errors.New("boom")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	next := updated.(Model)

	if next.state != stateError {
		t.Fatalf("state = %v, want %v", next.state, stateError)
	}

	if cmd == nil {
		t.Fatal("Update() returned nil command for ctrl+c while showing error")
	}

	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("ctrl+c command result = %#v, want %#v", msg, tea.Quit())
	}
}

func TestModelFiltersHosts(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{Alias: "web", Hostname: "web.example.com"},
		{Alias: "db", Hostname: "db.internal"},
		{Alias: "ops", Hostname: "bastion.EXAMPLE.com"},
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("example")})
	next := updated.(Model)

	if next.filter.Value() != "example" {
		t.Fatalf("filter.Value() = %q, want %q", next.filter.Value(), "example")
	}

	if len(next.visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2", len(next.visible))
	}

	if next.visible[0].Alias != "web" || next.visible[1].Alias != "ops" {
		t.Fatalf("visible = %#v, want hosts matching alias or hostname case-insensitively", next.visible)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	next = updated.(Model)

	if len(next.visible) != 0 {
		t.Fatalf("len(visible) = %d, want 0 after unmatched filter", len(next.visible))
	}
}

func TestModelMovesSelection(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{Alias: "web"},
		{Alias: "db"},
		{Alias: "ops"},
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	next := updated.(Model)

	if next.selected != 1 {
		t.Fatalf("selected = %d, want 1 after down", next.selected)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)

	if next.selected != 2 {
		t.Fatalf("selected = %d, want 2 at bottom bound", next.selected)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)

	if next.selected != 2 {
		t.Fatalf("selected = %d, want to stay at lower bound", next.selected)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next = updated.(Model)

	if next.selected != 1 {
		t.Fatalf("selected = %d, want 1 after up", next.selected)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next = updated.(Model)

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next = updated.(Model)

	if next.selected != 0 {
		t.Fatalf("selected = %d, want to stay at upper bound", next.selected)
	}
}

func TestModelClampsSelectionAfterFilteringShrinksList(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{Alias: "web", Hostname: "web.example.com"},
		{Alias: "db", Hostname: "db.internal"},
		{Alias: "ops", Hostname: "ops.example.com"},
	})
	m.selected = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("example")})
	next := updated.(Model)

	if len(next.visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2", len(next.visible))
	}

	if next.selected != 1 {
		t.Fatalf("selected = %d, want clamped to 1", next.selected)
	}

	if next.visible[next.selected].Alias != "ops" {
		t.Fatalf("selected host = %#v, want ops", next.visible[next.selected])
	}
}

func TestModelNavigationIsSafeWhenNoHostsVisible(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{{Alias: "web"}})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("nomatch")})
	next := updated.(Model)

	if len(next.visible) != 0 {
		t.Fatalf("len(visible) = %d, want 0", len(next.visible))
	}

	updated, cmd := next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)
	if cmd != nil {
		t.Fatal("down on empty list returned unexpected command")
	}
	if next.selected != 0 {
		t.Fatalf("selected = %d, want 0", next.selected)
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next = updated.(Model)
	if cmd != nil {
		t.Fatal("up on empty list returned unexpected command")
	}
	if next.selected != 0 {
		t.Fatalf("selected = %d, want 0", next.selected)
	}
}

func TestModelClearingFilterRestoresAllHosts(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{Alias: "web", Hostname: "web.example.com"},
		{Alias: "db", Hostname: "db.internal"},
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" web ")})
	next := updated.(Model)

	if len(next.visible) != 1 || next.visible[0].Alias != "web" {
		t.Fatalf("visible = %#v, want trimmed filter match", next.visible)
	}

	for i := 0; i < len(" web "); i++ {
		updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		next = updated.(Model)
	}

	if next.filter.Value() != "" {
		t.Fatalf("filter.Value() = %q, want empty", next.filter.Value())
	}

	if len(next.visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2 after clearing filter", len(next.visible))
	}

	if next.visible[0].Alias != "web" || next.visible[1].Alias != "db" {
		t.Fatalf("visible = %#v, want original host order restored", next.visible)
	}
}

func TestModelEscQuitsWhenReady(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{{Alias: "web"}})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if cmd == nil {
		t.Fatal("Update() returned nil command for esc")
	}

	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("esc command result = %#v, want %#v", msg, tea.Quit())
	}
}

func TestModelCtrlCQuitsWhenReady(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{{Alias: "web"}})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	next := updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if cmd == nil {
		t.Fatal("Update() returned nil command for ctrl+c")
	}

	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("ctrl+c command result = %#v, want %#v", msg, tea.Quit())
	}
}

func TestModelViewShowsSelectedHostDetails(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{
			Alias:         "web",
			Hostname:      "web.example.com",
			User:          "alice",
			Port:          "2222",
			ProxyJump:     "bastion",
			IdentityFiles: []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"},
		},
		{Alias: "db", Hostname: "db.internal"},
	})

	view := m.View()

	for _, want := range []string{
		"Portico",
		"Filter hosts",
		"Filter by alias or hostname",
		"Selected host",
		"> web",
		"db",
		"Alias: web",
		"Hostname: web.example.com",
		"User: alice",
		"Port: 2222",
		"ProxyJump: bastion",
		"IdentityFiles: ~/.ssh/id_ed25519, ~/.ssh/id_rsa",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want substring %q", view, want)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("nomatch")})
	filtered := updated.(Model)
	view = filtered.View()

	if !strings.Contains(view, "No hosts match the current filter.") {
		t.Fatalf("View() = %q, want empty filtered state", view)
	}

	if !strings.Contains(view, "up/down") {
		t.Fatalf("View() = %q, want footer help text", view)
	}
}

func TestModelViewUsesSplitLayoutOnWideTerminals(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{
			Alias:    "web",
			Hostname: "web.example.com",
			User:     "alice",
		},
		{Alias: "db", Hostname: "db.internal"},
	})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next := updated.(Model)

	if !next.useWideLayout() {
		t.Fatal("useWideLayout() = false, want true at threshold width")
	}

	split := next.renderSplitLayout()
	lines := strings.Split(split, "\n")
	if len(lines) < 2 {
		t.Fatalf("renderSplitLayout() = %q, want multiple lines", split)
	}
	if !strings.Contains(lines[1], "Hosts") || !strings.Contains(lines[1], "Details") {
		t.Fatalf("renderSplitLayout() = %q, want side-by-side panel titles on the same row", split)
	}

	view := next.View()
	for _, want := range []string{
		"Hosts",
		"Details",
		"Alias: web",
		"Hostname: web.example.com",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want substring %q", view, want)
		}
	}
}

func TestModelWindowSizeUpdatesLayoutState(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{{Alias: "web"}})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 99, Height: 24})
	next := updated.(Model)

	if next.width != 99 {
		t.Fatalf("width = %d, want 99", next.width)
	}

	if next.filter.Width != 97 {
		t.Fatalf("filter.Width = %d, want 97", next.filter.Width)
	}

	if next.useWideLayout() {
		t.Fatal("useWideLayout() = true, want false below threshold")
	}

	view := next.View()
	if strings.Contains(view, "Hosts") || strings.Contains(view, "Details") {
		t.Fatalf("View() = %q, want stacked layout below threshold", view)
	}
}

func TestModelSanitizesRenderedHostValues(t *testing.T) {
	m := readyModelForTest([]sshconfig.Host{
		{
			Alias:         "web\x1b[31m\u202E",
			Hostname:      "host\nname\t\a",
			User:          "ali\x00ce\u2066",
			Port:          "22\r22",
			ProxyJump:     "jump\x1b]8;;bad\u200d",
			IdentityFiles: []string{"id\x1b[0m", "two\nlines\u202c"},
		},
	})

	view := m.View()

	for _, forbidden := range []string{"\x1b", "\a", "\x00", "\r", "\u202e", "\u2066", "\u200d", "\u202c"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("View() = %q, should not contain control sequence %q", view, forbidden)
		}
	}

	for _, want := range []string{
		"Alias: web[31m",
		"Hostname: host name",
		"User: ali ce",
		"Port: 22 22",
		"ProxyJump: jump]8;;bad",
		"IdentityFiles: id[0m, two lines",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want sanitized substring %q", view, want)
		}
	}
}

func TestModelSanitizesRenderedError(t *testing.T) {
	m := New(Dependencies{Version: "dev"})
	m.state = stateError
	m.err = errors.New("boom\x1b[31m\a\u202e")

	view := m.View()

	if strings.Contains(view, "\x1b") || strings.Contains(view, "\a") || strings.Contains(view, "\u202e") {
		t.Fatalf("View() = %q, should not contain raw control sequences", view)
	}

	if !strings.Contains(view, "boom[31m") {
		t.Fatalf("View() = %q, want sanitized error text", view)
	}
}

func TestModelStartsCreateModeFromReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
	})

	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}

	if next.editPath != configPath {
		t.Fatalf("editPath = %q, want %q", next.editPath, configPath)
	}

	if next.draft.Alias != "" || next.draft.Hostname != "" {
		t.Fatalf("draft = %#v, want blank create draft", next.draft)
	}

	if next.editMode != editModeCreate {
		t.Fatalf("editMode = %v, want %v", next.editMode, editModeCreate)
	}
}

func TestModelPreviewsAndSavesCreatedHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
	})

	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	next = updated.(Model)

	next.editorFields[0].SetValue("api")
	next.editorFields[1].SetValue("api.example.com")
	next.editorFields[2].SetValue("svc")
	next.editorFields[3].SetValue("2222")
	next.editorFields[4].SetValue("bastion")
	next.editorFields[5].SetValue("~/.ssh/id_ed25519, ~/.ssh/id_rsa")
	next.syncDraftFromEditor()

	updated, _ = next.preparePreview()
	next = updated.(Model)

	if next.state != statePreview {
		t.Fatalf("state = %v, want %v", next.state, statePreview)
	}

	for _, want := range []string{"Host api", `HostName "api.example.com"`, `User "svc"`, `IdentityFile "~/.ssh/id_ed25519"`} {
		if !strings.Contains(next.preview, want) {
			t.Fatalf("preview = %q, want substring %q", next.preview, want)
		}
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	next = updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if len(next.visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2 after reload", len(next.visible))
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	text := string(got)
	for _, want := range []string{"Host api", `HostName "api.example.com"`, `User "svc"`, `Port "2222"`, `ProxyJump "bastion"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config = %q, want substring %q", text, want)
		}
	}
}

func TestModelKeepsEditorOpenOnBlankAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(configPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	next = updated.(Model)

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}

	if next.editErr == nil || !strings.Contains(next.editErr.Error(), "alias is required") {
		t.Fatalf("editErr = %v, want alias validation error", next.editErr)
	}
}

func TestModelKeepsEditorOpenOnDuplicateAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n\nHost db\n  HostName db.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(configPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	next = updated.(Model)
	next.editorFields[0].SetValue("db")
	next.syncDraftFromEditor()
	updated, _ = next.preparePreview()
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}

	if next.editErr == nil || !strings.Contains(next.editErr.Error(), "already exists") {
		t.Fatalf("editErr = %v, want duplicate alias validation error", next.editErr)
	}
}

func TestModelNavigatesEditorWithArrowKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(configPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	next = updated.(Model)

	if next.editorIndex != 0 {
		t.Fatalf("editorIndex = %d, want 0", next.editorIndex)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)
	if next.editorIndex != 1 {
		t.Fatalf("editorIndex = %d, want 1 after down", next.editorIndex)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next = updated.(Model)
	if next.editorIndex != 0 {
		t.Fatalf("editorIndex = %d, want 0 after up", next.editorIndex)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyTab})
	next = updated.(Model)
	if next.editorIndex != 0 {
		t.Fatalf("editorIndex = %d, want tab to do nothing", next.editorIndex)
	}
}

func TestModelRejectsDuplicateAliasAcrossLoadedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}

	rootPath := filepath.Join(configDir, "config")
	childPath := filepath.Join(configDir, "shared.config")
	if err := os.WriteFile(childPath, []byte("Host shared\n  HostName shared.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}
	if err := os.WriteFile(rootPath, []byte("Include shared.config\nHost root\n  HostName root.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(rootPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	next = updated.(Model)
	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}

	next.editorFields[0].SetValue("root")
	next.syncDraftFromEditor()
	updated, _ = next.preparePreview()
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}
	if next.editErr == nil || !strings.Contains(next.editErr.Error(), "already exists") {
		t.Fatalf("editErr = %v, want cross-file duplicate alias validation error", next.editErr)
	}
}

func TestModelReturnsToEditorFromPreviewOnEscape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(configPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	next = updated.(Model)
	next.editorFields[0].SetValue("api")
	next.editorFields[1].SetValue("api.example.com")
	next.syncDraftFromEditor()
	updated, _ = next.preparePreview()
	next = updated.(Model)

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}
}

func TestModelDeletesHostAfterConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n\nHost db\n  HostName db.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
	})

	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	next = updated.(Model)

	if next.state != stateConfirmDelete {
		t.Fatalf("state = %v, want %v", next.state, stateConfirmDelete)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next = updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	if len(next.visible) != 1 {
		t.Fatalf("len(visible) = %d, want 1 after delete reload", len(next.visible))
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	text := string(got)
	if strings.Contains(text, "Host db") {
		t.Fatalf("saved config = %q, want db host removed", text)
	}
	if !strings.Contains(text, "Host web") {
		t.Fatalf("saved config = %q, want remaining host kept", text)
	}
}

func TestModelCancelsDeleteWithEscape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	original := "Host web\n  HostName web.example.com\n\nHost db\n  HostName db.example.com\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{Version: "dev", LoadHosts: func() (sshconfig.Result, error) { return sshconfig.Load(configPath) }})
	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next = updated.(Model)

	if next.state != stateReady {
		t.Fatalf("state = %v, want %v", next.state, stateReady)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	if string(got) != original {
		t.Fatalf("saved config = %q, want unchanged", string(got))
	}
}

func TestModelEditsExistingHostAndSavesIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte("Host web\n  HostName web.example.com\n  User root\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}

	m := New(Dependencies{
		Version: "dev",
		LoadHosts: func() (sshconfig.Result, error) {
			return sshconfig.Load(configPath)
		},
	})

	updated, _ := m.Update(m.Init()())
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	next = updated.(Model)

	if next.state != stateEditing {
		t.Fatalf("state = %v, want %v", next.state, stateEditing)
	}

	next.editorFields[0].SetValue("api")
	next.editorFields[1].SetValue("web.internal")
	next.editorFields[2].SetValue("alice")
	next.syncDraftFromEditor()

	updated, _ = next.preparePreview()
	next = updated.(Model)
	updated, _ = next.savePreview()
	next = updated.(Model)

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	text := string(got)
	for _, want := range []string{"Host api", `HostName "web.internal"`, `User "alice"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config = %q, want substring %q", text, want)
		}
	}
	if strings.Contains(text, "Host web") {
		t.Fatalf("saved config = %q, want renamed host only", text)
	}
}

func readyModelForTest(hosts []sshconfig.Host) Model {
	m := New(Dependencies{Version: "dev"})
	m.state = stateReady
	m.hosts = append([]sshconfig.Host(nil), hosts...)
	m.visible = append([]sshconfig.Host(nil), hosts...)
	return m
}
