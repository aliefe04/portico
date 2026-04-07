package sshconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadBasicHosts(t *testing.T) {
	path := fixturePath("basic.config")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:         "web",
				Patterns:      []string{"web"},
				Hostname:      "web.example.com",
				User:          "root",
				Port:          "2222",
				IdentityFiles: []string{"~/.ssh/id_ed25519"},
				SourcePath:    path,
			},
			{
				Alias:      "db",
				Patterns:   []string{"db"},
				Hostname:   "db.example.com",
				User:       "admin",
				SourcePath: path,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func TestLoadWildcardHost(t *testing.T) {
	path := fixturePath("wildcards.config")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:      "*.corp",
				Patterns:   []string{"*.corp"},
				Hostname:   "*.corp",
				User:       "deploy",
				Port:       "2200",
				SourcePath: path,
				Wildcard:   true,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func TestLoadIncludedHosts(t *testing.T) {
	path := fixturePath("included-root.config")
	includedPath := fixturePath("included-extra.config")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:      "cache",
				Patterns:   []string{"cache"},
				Hostname:   "cache.example.com",
				User:       "redis",
				SourcePath: includedPath,
			},
			{
				Alias:      "bastion",
				Patterns:   []string{"bastion"},
				Hostname:   "bastion.example.com",
				User:       "ops",
				SourcePath: path,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func TestLoadIncludedHostsExpandsTildePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(configDir, "extras"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	rootPath := filepath.Join(configDir, "root.config")
	childPath := filepath.Join(configDir, "extras", "child.config")

	if err := os.WriteFile(rootPath, []byte("Include ~/.ssh/extras/child.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	if err := os.WriteFile(childPath, []byte("Host cache\n  HostName cache.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{
			{
				Alias:      "cache",
				Patterns:   []string{"cache"},
				Hostname:   "cache.example.com",
				SourcePath: childPath,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadAllowsRelativeIncludeOutsideEntryFileDirectoryWithinSSHHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".ssh")
	rootDir := filepath.Join(configDir, "profiles", "work")
	sharedPath := filepath.Join(configDir, "shared.config")
	rootPath := filepath.Join(rootDir, "root.config")

	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(sharedPath, []byte("Host shared\n  HostName shared.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", sharedPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include ../../shared.config\nHost root\n  HostName root.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	sharedRealPath, err := filepath.EvalSymlinks(sharedPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", sharedPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{
			{
				Alias:      "shared",
				Patterns:   []string{"shared"},
				Hostname:   "shared.example.com",
				SourcePath: sharedRealPath,
			},
			{
				Alias:      "root",
				Patterns:   []string{"root"},
				Hostname:   "root.example.com",
				SourcePath: rootPath,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadAllowsCrossRootIncludeBetweenStandardSSHRoots(t *testing.T) {
	home := t.TempDir()
	fakeSystemRoot := filepath.Join(t.TempDir(), "etc", "ssh")
	t.Setenv("HOME", home)

	originalSystemRootDir := systemSSHRootDir
	systemSSHRootDir = func() string { return fakeSystemRoot }
	t.Cleanup(func() {
		systemSSHRootDir = originalSystemRootDir
	})

	userRoot := filepath.Join(home, ".ssh")
	rootPath := filepath.Join(userRoot, "root.config")
	childPath := filepath.Join(fakeSystemRoot, "shared.conf")

	if err := os.MkdirAll(userRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", userRoot, err)
	}

	if err := os.MkdirAll(fakeSystemRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", fakeSystemRoot, err)
	}

	if err := os.WriteFile(childPath, []byte("Host shared\n  HostName shared.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include "+childPath+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{{
			Alias:      "shared",
			Patterns:   []string{"shared"},
			Hostname:   "shared.example.com",
			SourcePath: childPath,
		}},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadAllowsHomeRootIncludeFromSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".ssh")
	orbStackDir := filepath.Join(home, ".orbstack", "ssh")
	rootPath := filepath.Join(configDir, "config")
	childPath := filepath.Join(orbStackDir, "config")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}

	if err := os.MkdirAll(orbStackDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", orbStackDir, err)
	}

	if err := os.WriteFile(childPath, []byte("Host orb\n  HostName orb.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include ~/.orbstack/ssh/config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}
	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{{
			Alias:      "orb",
			Patterns:   []string{"orb"},
			Hostname:   "orb.example.com",
			SourcePath: childPath,
		}},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadAppliesParentDefaultsToIncludedHosts(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "root.config")
	childPath := filepath.Join(root, "child.config")
	childRealPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", root, err)
	}
	childRealPath = filepath.Join(childRealPath, "child.config")

	if err := os.WriteFile(childPath, []byte("Host cache\n  HostName cache.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	contents := strings.Join([]string{
		"Host *",
		"  User alice",
		"  Include child.config",
	}, "\n") + "\n"
	if err := os.WriteFile(rootPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{
			{
				Alias:      "cache",
				Patterns:   []string{"cache"},
				Hostname:   "cache.example.com",
				User:       "alice",
				SourcePath: childRealPath,
			},
			{
				Alias:      "*",
				Patterns:   []string{"*"},
				Hostname:   "*",
				User:       "alice",
				SourcePath: rootPath,
				Wildcard:   true,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadResolvesInheritedHostValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inherited.config")
	contents := strings.Join([]string{
		"Host *",
		"  User ali",
		"  Port 22",
		"  IdentityFile ~/.ssh/id_default",
		"Host *.example.com",
		"  ProxyJump bastion",
		"  IdentityFile ~/.ssh/id_wild",
		"Host web.example.com",
		"  HostName real.example.com",
	}, "\n") + "\n"

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:         "*",
				Patterns:      []string{"*"},
				Hostname:      "*",
				User:          "ali",
				Port:          "22",
				IdentityFiles: []string{"~/.ssh/id_default"},
				SourcePath:    path,
				Wildcard:      true,
			},
			{
				Alias:         "*.example.com",
				Patterns:      []string{"*.example.com"},
				Hostname:      "*.example.com",
				User:          "ali",
				Port:          "22",
				IdentityFiles: []string{"~/.ssh/id_default", "~/.ssh/id_wild"},
				ProxyJump:     "bastion",
				SourcePath:    path,
				Wildcard:      true,
			},
			{
				Alias:         "web.example.com",
				Patterns:      []string{"web.example.com"},
				Hostname:      "real.example.com",
				User:          "ali",
				Port:          "22",
				IdentityFiles: []string{"~/.ssh/id_default", "~/.ssh/id_wild"},
				ProxyJump:     "bastion",
				SourcePath:    path,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func TestLoadRewritesIncludeWithTabWhitespace(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "root.config")
	childPath := filepath.Join(root, "child.config")
	childRealPath := childPath

	if err := os.WriteFile(childPath, []byte("Host child\n  HostName child.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include\tchild.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	var err error
	childRealPath, err = filepath.EvalSymlinks(childPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", childPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{
			{
				Alias:      "child",
				Patterns:   []string{"child"},
				Hostname:   "child.example.com",
				SourcePath: childRealPath,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadNestedIncludePreservesRelativeSourcePath(t *testing.T) {
	path := fixturePath(filepath.Join("nested", "root.config"))
	childPath := fixturePath(filepath.Join("nested", "subdir", "child.config"))
	grandchildPath := fixturePath(filepath.Join("nested", "subdir", "deeper", "grandchild.config"))

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:      "grandchild",
				Patterns:   []string{"grandchild"},
				Hostname:   "grandchild.example.com",
				SourcePath: grandchildPath,
			},
			{
				Alias:      "child",
				Patterns:   []string{"child"},
				Hostname:   "child.example.com",
				SourcePath: childPath,
			},
			{
				Alias:      "root",
				Patterns:   []string{"root"},
				Hostname:   "root.example.com",
				SourcePath: path,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func TestLoadRejectsParentTraversalInclude(t *testing.T) {
	root := t.TempDir()
	outsidePath := filepath.Join(root, "outside.config")
	configDir := filepath.Join(root, "configs")
	path := filepath.Join(configDir, "root.config")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(outsidePath, []byte("Host escape\n  HostName escape.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", outsidePath, err)
	}

	if err := os.WriteFile(path, []byte("Include ../outside.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unsafe include") {
		t.Fatalf("Load(%q) error = %v, want unsafe include error", path, err)
	}
}

func TestLoadRejectsNonRegularIncludeTarget(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "root.config")
	childDir := filepath.Join(root, "child.d")

	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(rootPath, []byte("Include child.d\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	_, err := Load(rootPath)
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("Load(%q) error = %v, want non-regular include error", rootPath, err)
	}
}

func TestLoadRejectsSymlinkEscapeInclude(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	configDir := filepath.Join(root, "configs")
	path := filepath.Join(configDir, "root.config")
	symlinkPath := filepath.Join(configDir, "linked.config")
	targetPath := filepath.Join(outsideDir, "outside.config")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(targetPath, []byte("Host escape\n  HostName escape.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", targetPath, err)
	}

	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("Symlink(%q, %q) error = %v", targetPath, symlinkPath, err)
	}

	if err := os.WriteFile(path, []byte("Include linked.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unsafe include") {
		t.Fatalf("Load(%q) error = %v, want unsafe include error", path, err)
	}
}

func TestLoadRejectsRecursiveIncludeCycle(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.config")
	secondPath := filepath.Join(root, "second.config")

	if err := os.WriteFile(firstPath, []byte("Include second.config\nHost first\n  HostName first.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", firstPath, err)
	}

	if err := os.WriteFile(secondPath, []byte("Include first.config\nHost second\n  HostName second.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", secondPath, err)
	}

	_, err := Load(firstPath)
	if err == nil || !strings.Contains(err.Error(), "include cycle") {
		t.Fatalf("Load(%q) error = %v, want include cycle error", firstPath, err)
	}
}

func TestLoadDeduplicatesGlobbedIncludes(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "root.config")
	firstPath := filepath.Join(root, "child-a.config")
	secondPath := filepath.Join(root, "child-b.config")

	if err := os.WriteFile(firstPath, []byte("Host first\n  HostName first.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", firstPath, err)
	}

	if err := os.WriteFile(secondPath, []byte("Host second\n  HostName second.example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", secondPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include child-*.config child-a.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	firstRealPath, err := filepath.EvalSymlinks(firstPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", firstPath, err)
	}

	secondRealPath, err := filepath.EvalSymlinks(secondPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", secondPath, err)
	}

	got, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", rootPath, err)
	}

	want := Result{
		Path: rootPath,
		Hosts: []Host{
			{
				Alias:      "first",
				Patterns:   []string{"first"},
				Hostname:   "first.example.com",
				SourcePath: firstRealPath,
			},
			{
				Alias:      "second",
				Patterns:   []string{"second"},
				Hostname:   "second.example.com",
				SourcePath: secondRealPath,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", rootPath, got, want)
	}
}

func TestLoadMalformedConfigReturnsError(t *testing.T) {
	path := fixturePath("malformed.config")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load(%q) error = nil, want non-nil", path)
	}

	if !strings.Contains(err.Error(), "HostName") || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("Load(%q) error = %v, want HostName empty value error", path, err)
	}
}

func TestLoadRejectsOversizedRootConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.config")
	contents := []byte(strings.Repeat("#", 1024*1024+1))

	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Load(%q) error = %v, want too large error", path, err)
	}
}

func TestLoadRejectsOversizedIncludedConfig(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "root.config")
	childPath := filepath.Join(root, "child.config")

	if err := os.WriteFile(childPath, []byte(strings.Repeat("#", 1024*1024+1)), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", childPath, err)
	}

	if err := os.WriteFile(rootPath, []byte("Include child.config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", rootPath, err)
	}

	_, err := Load(rootPath)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Load(%q) error = %v, want too large error", rootPath, err)
	}
}

func TestLoadAllowsOtherEmptyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-user.config")
	contents := "Host sample\n  User\n"

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	want := Result{
		Path: path,
		Hosts: []Host{
			{
				Alias:      "sample",
				Patterns:   []string{"sample"},
				Hostname:   "sample",
				SourcePath: path,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load(%q) = %#v, want %#v", path, got, want)
	}
}

func fixturePath(name string) string {
	return filepath.Join("testdata", "configs", name)
}
