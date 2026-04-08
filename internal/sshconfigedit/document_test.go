package sshconfigedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentCanCreateUpdateDeleteAndSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	contents := strings.Join([]string{
		"# keep this comment",
		"Host web",
		"  HostName web.example.com",
		"  User root",
		"",
		"Host db",
		"  HostName db.example.com",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", path, err)
	}

	if err := doc.UpdateHost("web", HostDraft{Alias: "web", Hostname: "web.internal", User: "alice", Port: "2222", IdentityFiles: []string{"~/.ssh/id_ed25519"}}); err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}

	if err := doc.CreateHost(HostDraft{Alias: "api", Hostname: "api.example.com", User: "svc"}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err := doc.DeleteHost("db"); err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}

	if err := doc.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	backupMatches, err := filepath.Glob(path + ".bak*")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(backupMatches) == 0 {
		t.Fatal("Save() did not create a backup")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	text := string(got)
	for _, want := range []string{
		"# keep this comment",
		"Host web",
		"  HostName \"web.internal\"",
		"  User \"alice\"",
		"  Port \"2222\"",
		"  IdentityFile \"~/.ssh/id_ed25519\"",
		"Host api",
		"  HostName \"api.example.com\"",
		"  User \"svc\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config = %q, want substring %q", text, want)
		}
	}
	if strings.Contains(text, "Host db") {
		t.Fatalf("saved config = %q, want db host removed", text)
	}

	if _, err := LoadDocument(path); err != nil {
		t.Fatalf("round-trip LoadDocument(%q) error = %v", path, err)
	}
}

func TestDocumentRejectsDuplicateAliasOnRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	contents := "Host web\n  HostName web.example.com\n\nHost db\n  HostName db.example.com\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", path, err)
	}

	err = doc.UpdateHost("web", HostDraft{Alias: "db", Hostname: "web.internal"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("UpdateHost() error = %v, want duplicate alias error", err)
	}
}

func TestDocumentRejectsStaleSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	contents := "Host web\n  HostName web.example.com\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", path, err)
	}

	if err := os.WriteFile(path, []byte("Host web\n  HostName changed.example.com\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	if err := doc.Save(); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("Save() error = %v, want stale save rejection", err)
	}
}

func TestDocumentPreservesOriginalKeyCasingOnUpdate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	contents := "Host heltz-1\n  Hostname 100.64.1.4\n  User root\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", path, err)
	}

	if err := doc.UpdateHost("heltz-1", HostDraft{Alias: "heltz-1", Hostname: "100.64.1.5", User: "root"}); err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}

	if err := doc.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	text := string(got)
	if !strings.Contains(text, "Hostname \"100.64.1.5\"") {
		t.Fatalf("saved config = %q, want preserved Hostname casing", text)
	}
	if strings.Contains(text, "HostName \"100.64.1.5\"") {
		t.Fatalf("saved config = %q, want not normalized to HostName", text)
	}
}

func TestDocumentAddsPorticoCommentAndBlankLineWhenCreatingHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	contents := "Host web\n  HostName web.example.com\n  User root\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", path, err)
	}

	if err := doc.CreateHost(HostDraft{Alias: "api", Hostname: "api.example.com", User: "svc"}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err := doc.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	text := string(got)
	for _, want := range []string{
		"# Added with Portico",
		"User root\n# Added with Portico\nHost api\n",
		"Host api\n  HostName \"api.example.com\"\n  User \"svc\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config = %q, want substring %q", text, want)
		}
	}
}

func TestDraftSnippetQuotesValuesWithWhitespace(t *testing.T) {
	got := draftSnippet(HostDraft{
		Alias:         "web",
		Hostname:      "web example.com",
		User:          "alice smith",
		Port:          "2200",
		ProxyJump:     "jump host",
		IdentityFiles: []string{"~/.ssh/id ed25519", "/tmp/key\tone"},
	}, managedKeyNames(nil))

	for _, want := range []string{
		`HostName "web example.com"`,
		`User "alice smith"`,
		`Port "2200"`,
		`ProxyJump "jump host"`,
		`IdentityFile "~/.ssh/id ed25519"`,
		`IdentityFile "/tmp/key\tone"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("draftSnippet() = %q, want substring %q", got, want)
		}
	}
}
