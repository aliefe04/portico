package platform

import (
	"path/filepath"
	"testing"
)

func TestDefaultSSHConfigPath(t *testing.T) {
	t.Run("returns config path from home", func(t *testing.T) {
		got := DefaultSSHConfigPath("/tmp/alice")
		want := filepath.Join("/tmp/alice", ".ssh", "config")

		if got != want {
			t.Fatalf("DefaultSSHConfigPath(/tmp/alice) = %q, want %q", got, want)
		}
	})

	t.Run("returns empty string for empty home", func(t *testing.T) {
		got := DefaultSSHConfigPath("")

		if got != "" {
			t.Fatalf("DefaultSSHConfigPath(\"\") = %q, want %q", got, "")
		}
	})
}
