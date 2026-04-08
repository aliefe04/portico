package sshconfigedit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	ssh "github.com/kevinburke/ssh_config"
)

type HostDraft struct {
	Alias         string
	Hostname      string
	User          string
	Port          string
	ProxyJump     string
	IdentityFiles []string
}

type Document struct {
	Path    string
	raw     []byte
	cfg     *ssh.Config
	mode    os.FileMode
	modTime time.Time
	size    int64
	dev     uint64
	ino     uint64
}

func LoadDocument(path string) (*Document, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(realPath)
	if err != nil {
		return nil, err
	}

	cfg, err := ssh.DecodeBytes(raw)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(realPath)
	if err != nil {
		return nil, err
	}
	var dev, ino uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		dev = uint64(stat.Dev)
		ino = uint64(stat.Ino)
	}

	return &Document{
		Path:    realPath,
		raw:     append([]byte(nil), raw...),
		cfg:     cfg,
		mode:    info.Mode(),
		modTime: info.ModTime(),
		size:    info.Size(),
		dev:     dev,
		ino:     ino,
	}, nil
}

func (d *Document) String() string {
	if d == nil || d.cfg == nil {
		return ""
	}

	return d.cfg.String()
}

func (d *Document) Preview() string {
	if d == nil {
		return ""
	}

	return unifiedPreview(string(d.raw), d.String())
}

func (d *Document) UpdateHost(alias string, draft HostDraft) error {
	host, err := d.findHost(alias)
	if err != nil {
		return err
	}

	trimmedAlias := strings.TrimSpace(draft.Alias)
	if trimmedAlias == "" {
		return fmt.Errorf("sshconfigedit: alias is required")
	}
	if trimmedAlias != alias {
		if _, err := d.findHostIndex(trimmedAlias); err == nil {
			return fmt.Errorf("sshconfigedit: host %q already exists", trimmedAlias)
		}
	}

	updated, err := hostFromDraft(draft, host)
	if err != nil {
		return err
	}

	host.Patterns = updated.Patterns
	host.Nodes = mergeManagedNodes(host.Nodes, updated.Nodes)
	host.EOLComment = updated.EOLComment
	return nil
}

func (d *Document) CreateHost(draft HostDraft) error {
	if strings.TrimSpace(draft.Alias) == "" {
		return fmt.Errorf("sshconfigedit: alias is required")
	}

	if _, err := d.findHost(draft.Alias); err == nil {
		return fmt.Errorf("sshconfigedit: host %q already exists", draft.Alias)
	}

	host, err := hostFromDraft(draft, nil)
	if err != nil {
		return err
	}

	if len(d.cfg.Hosts) > 0 {
		appendPorticoSeparator(d.cfg.Hosts[len(d.cfg.Hosts)-1])
	}
	d.cfg.Hosts = append(d.cfg.Hosts, host)
	return nil
}

func (d *Document) DeleteHost(alias string) error {
	idx, err := d.findHostIndex(alias)
	if err != nil {
		return err
	}

	d.cfg.Hosts = append(d.cfg.Hosts[:idx], d.cfg.Hosts[idx+1:]...)
	return nil
}

func (d *Document) Save() error {
	if err := d.ensureWritable(); err != nil {
		return err
	}

	current, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, d.raw) {
		return fmt.Errorf("sshconfigedit: config changed on disk since it was loaded")
	}

	rendered := []byte(d.String())
	if _, err := ssh.DecodeBytes(rendered); err != nil {
		return fmt.Errorf("sshconfigedit: rendered config invalid: %w", err)
	}

	backupPath, err := writeBackup(d.Path, current, d.mode)
	if err != nil {
		return err
	}

	if err := writeAtomic(d.Path, rendered, d.mode); err != nil {
		return err
	}

	verified, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	if _, err := ssh.DecodeBytes(verified); err != nil {
		if restoreErr := writeAtomic(d.Path, current, d.mode); restoreErr != nil {
			return fmt.Errorf("sshconfigedit: verify failed: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("sshconfigedit: verify failed: %w (restored from %s)", err, backupPath)
	}

	info, err := os.Stat(d.Path)
	if err != nil {
		return err
	}

	d.raw = append([]byte(nil), rendered...)
	d.modTime = info.ModTime()
	d.size = info.Size()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		d.dev = uint64(stat.Dev)
		d.ino = uint64(stat.Ino)
	}
	return nil
}

func (d *Document) findHost(alias string) (*ssh.Host, error) {
	idx, err := d.findHostIndex(alias)
	if err != nil {
		return nil, err
	}
	return d.cfg.Hosts[idx], nil
}

func (d *Document) findHostIndex(alias string) (int, error) {
	trimmed := strings.TrimSpace(alias)
	if trimmed == "" {
		return -1, fmt.Errorf("sshconfigedit: alias is required")
	}

	for i, host := range d.cfg.Hosts {
		if hostAlias(host) == trimmed {
			return i, nil
		}
	}

	return -1, fmt.Errorf("sshconfigedit: host %q not found", alias)
}

func hostFromDraft(draft HostDraft, template *ssh.Host) (*ssh.Host, error) {
	snippet := draftSnippet(draft, managedKeyNames(template))
	cfg, err := ssh.DecodeBytes([]byte(snippet))
	if err != nil {
		return nil, err
	}
	for _, host := range cfg.Hosts {
		if hostAlias(host) == strings.TrimSpace(draft.Alias) {
			return host, nil
		}
	}
	return nil, fmt.Errorf("sshconfigedit: generated host block %q not found", draft.Alias)
}

func hostAlias(host *ssh.Host) string {
	patterns := make([]string, 0, len(host.Patterns))
	for _, pattern := range host.Patterns {
		patterns = append(patterns, pattern.String())
	}
	return strings.Join(patterns, " ")
}

func mergeManagedNodes(nodes []ssh.Node, replacement []ssh.Node) []ssh.Node {
	out := make([]ssh.Node, 0, len(nodes)+4)
	managed := map[string]struct{}{
		"HostName":     {},
		"User":         {},
		"Port":         {},
		"ProxyJump":    {},
		"IdentityFile": {},
	}

	for _, node := range nodes {
		kv, ok := node.(*ssh.KV)
		if !ok {
			out = append(out, node)
			continue
		}
		if _, ok := managed[kv.Key]; ok || managedKeyMatch(kv.Key) {
			continue
		}
		out = append(out, node)
	}
	out = append(out, replacement...)

	return out
}

func draftSnippet(draft HostDraft, keys map[string]string) string {
	parts := make([]string, 0, 6)
	parts = append(parts, "Host "+strings.TrimSpace(draft.Alias))
	appendLine := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parts = append(parts, "  "+key+" "+strconv.Quote(value))
	}

	appendLine(keys["HostName"], draft.Hostname)
	appendLine(keys["User"], draft.User)
	appendLine(keys["Port"], draft.Port)
	appendLine(keys["ProxyJump"], draft.ProxyJump)
	for _, file := range draft.IdentityFiles {
		appendLine(keys["IdentityFile"], file)
	}

	return strings.Join(parts, "\n") + "\n"
}

func managedKeyNames(template *ssh.Host) map[string]string {
	keys := map[string]string{
		"HostName":     "HostName",
		"User":         "User",
		"Port":         "Port",
		"ProxyJump":    "ProxyJump",
		"IdentityFile": "IdentityFile",
	}
	if template == nil {
		return keys
	}

	for _, node := range template.Nodes {
		kv, ok := node.(*ssh.KV)
		if !ok {
			continue
		}
		for canonical := range keys {
			if strings.EqualFold(kv.Key, canonical) {
				keys[canonical] = kv.Key
			}
		}
	}

	return keys
}

func managedKeyMatch(key string) bool {
	for _, managed := range []string{"HostName", "User", "Port", "ProxyJump", "IdentityFile"} {
		if strings.EqualFold(key, managed) {
			return true
		}
	}
	return false
}

func appendPorticoSeparator(host *ssh.Host) {
	host.Nodes = append(host.Nodes, &ssh.Empty{Comment: " Added with Portico"})
}

func writeBackup(path string, contents []byte, mode os.FileMode) (string, error) {
	backupPath := fmt.Sprintf("%s.bak.%s", path, time.Now().UTC().Format("20060102T150405.000000000Z"))
	f, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return "", err
	}
	if _, err := f.Write(contents); err != nil {
		_ = f.Close()
		_ = os.Remove(backupPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(backupPath)
		return "", err
	}
	return backupPath, nil
}

func writeAtomic(path string, contents []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".portico-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(mode.Perm()); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(contents); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func (d *Document) ensureWritable() error {
	info, err := os.Lstat(d.Path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sshconfigedit: refusing to write symlink target %s", d.Path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("sshconfigedit: refusing to write non-regular target %s", d.Path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if d.dev != 0 && d.ino != 0 && (uint64(stat.Dev) != d.dev || uint64(stat.Ino) != d.ino) {
			return fmt.Errorf("sshconfigedit: config changed on disk since it was loaded")
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	homeReal, err := filepath.EvalSymlinks(home)
	if err != nil {
		homeReal = home
	}
	if !withinRoot(homeReal, d.Path) {
		return fmt.Errorf("sshconfigedit: refusing to write outside home directory: %s", d.Path)
	}
	return nil
}

func withinRoot(rootDir, path string) bool {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func unifiedPreview(before, after string) string {
	if before == after {
		return "(no changes)"
	}

	beforeLines := strings.Split(strings.TrimSuffix(before, "\n"), "\n")
	afterLines := strings.Split(strings.TrimSuffix(after, "\n"), "\n")
	out := []string{"--- current", "+++ proposed"}
	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			out = append(out, " "+beforeLines[i])
			i++
			j++
			continue
		}
		if i < len(beforeLines) {
			out = append(out, "-"+beforeLines[i])
			i++
		}
		if j < len(afterLines) {
			out = append(out, "+"+afterLines[j])
			j++
		}
	}
	return strings.Join(out, "\n")
}
