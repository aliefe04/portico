package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   []string
	}{
		{goos: "darwin", goarch: "arm64", want: []string{"portico_darwin_arm64.tar.gz", "portico_0.1.0_darwin_arm64.tar.gz"}},
		{goos: "darwin", goarch: "amd64", want: []string{"portico_darwin_amd64.tar.gz", "portico_0.1.0_darwin_amd64.tar.gz"}},
		{goos: "linux", goarch: "arm64", want: []string{"portico_linux_arm64.tar.gz", "portico_0.1.0_linux_arm64.tar.gz"}},
		{goos: "linux", goarch: "amd64", want: []string{"portico_linux_amd64.tar.gz", "portico_0.1.0_linux_amd64.tar.gz"}},
	}

	for _, tt := range tests {
		got, err := ArchiveNames("v0.1.0", tt.goos, tt.goarch)
		if err != nil {
			t.Fatalf("ArchiveNames(%q, %q) error = %v", tt.goos, tt.goarch, err)
		}
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Fatalf("ArchiveNames(%q, %q) = %q, want %q", tt.goos, tt.goarch, strings.Join(got, ","), strings.Join(tt.want, ","))
		}
	}
}

func TestArchiveNameRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := ArchiveNames("v0.1.0", "windows", "amd64"); err == nil {
		t.Fatal("ArchiveNames() error = nil, want unsupported platform error")
	}
}

func TestLatestTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/aliefe04/portico/releases/latest" {
			t.Fatalf("request path = %q, want latest release endpoint", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.0"}`))
	}))
	defer server.Close()

	got, err := LatestTag(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("LatestTag() error = %v", err)
	}
	if got != "v0.1.0" {
		t.Fatalf("LatestTag() = %q, want %q", got, "v0.1.0")
	}
}

func TestChecksumForAsset(t *testing.T) {
	checksums := strings.Join([]string{
		"aaaaaaaa portico_darwin_arm64.tar.gz",
		"bbbbbbbb checksums.txt",
	}, "\n")

	got, err := ChecksumForAsset([]byte(checksums), "portico_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("ChecksumForAsset() error = %v", err)
	}
	if got != "aaaaaaaa" {
		t.Fatalf("ChecksumForAsset() = %q, want %q", got, "aaaaaaaa")
	}
}

func TestExtractBinary(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"README.md": "docs",
		"portico":   "binary-bytes",
	})

	got, err := ExtractBinary(archive, BinaryName)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}
	if string(got) != "binary-bytes" {
		t.Fatalf("ExtractBinary() = %q, want %q", string(got), "binary-bytes")
	}
}

func TestReplaceExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portico")
	if err := os.WriteFile(path, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	if err := ReplaceExecutable(path, []byte("new-binary")); err != nil {
		t.Fatalf("ReplaceExecutable() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, string(got), "new-binary")
	}

	backups, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("len(backups) = %d, want 1", len(backups))
	}

	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", backups[0], err)
	}
	if string(backup) != "old-binary" {
		t.Fatalf("backup = %q, want %q", string(backup), "old-binary")
	}
}

func TestSelfUpdate(t *testing.T) {
	archive := testArchive(t, map[string]string{"portico": "new-binary"})
	sum := sha256sum(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/aliefe04/portico/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0"}`))
		case "/aliefe04/portico/releases/download/v0.2.0/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s %s\n", sum, "portico_darwin_arm64.tar.gz")
		case "/aliefe04/portico/releases/download/v0.2.0/portico_darwin_arm64.tar.gz":
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "portico")
	if err := os.WriteFile(path, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	result, err := (Updater{Client: server.Client(), APIBaseURL: server.URL, AssetBaseURL: server.URL}).SelfUpdate(context.Background(), "v0.1.0", path, "darwin", "arm64")
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.LatestVersion != "v0.2.0" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "v0.2.0")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("updated binary = %q, want %q", string(got), "new-binary")
	}
}

func TestSelfUpdateSupportsVersionedArchiveNames(t *testing.T) {
	archive := testArchive(t, map[string]string{"portico": "new-binary"})
	sum := sha256sum(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/aliefe04/portico/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.1.0"}`))
		case "/aliefe04/portico/releases/download/v0.1.0/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s %s\n", sum, "portico_0.1.0_darwin_arm64.tar.gz")
		case "/aliefe04/portico/releases/download/v0.1.0/portico_0.1.0_darwin_arm64.tar.gz":
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "portico")
	if err := os.WriteFile(path, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	result, err := (Updater{Client: server.Client(), APIBaseURL: server.URL, AssetBaseURL: server.URL}).SelfUpdate(context.Background(), "v0.0.9", path, "darwin", "arm64")
	if err != nil {
		t.Fatalf("SelfUpdate() error = %v", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	return buf.Bytes()
}

func TestAssetURL(t *testing.T) {
	got := AssetURL("https://github.com", "v0.1.0", "portico_darwin_arm64.tar.gz")
	want := "https://github.com/aliefe04/portico/releases/download/v0.1.0/portico_darwin_arm64.tar.gz"
	if got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
}

func TestChecksumsURL(t *testing.T) {
	got := ChecksumsURL("https://github.com", "v0.1.0")
	want := "https://github.com/aliefe04/portico/releases/download/v0.1.0/checksums.txt"
	if got != want {
		t.Fatalf("ChecksumsURL() = %q, want %q", got, want)
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := NormalizeVersion("0.1.0"); got != "v0.1.0" {
		t.Fatalf("NormalizeVersion() = %q, want %q", got, "v0.1.0")
	}
	if got := NormalizeVersion("v0.1.0"); got != "v0.1.0" {
		t.Fatalf("NormalizeVersion() = %q, want %q", got, "v0.1.0")
	}
}

func TestChecksumForAssetMissing(t *testing.T) {
	_, err := ChecksumForAsset([]byte("aaaaaaaa other.tar.gz\n"), "portico_darwin_arm64.tar.gz")
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("ChecksumForAsset() error = %v, want missing checksum error", err)
	}
}

func TestLatestTagRejectsMissingTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	_, err := LatestTag(context.Background(), server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "tag") {
		t.Fatalf("LatestTag() error = %v, want missing tag error", err)
	}
}

func sha256sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
