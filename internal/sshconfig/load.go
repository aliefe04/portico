package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ssh "github.com/kevinburke/ssh_config"
)

func Load(path string) (Result, error) {
	parsePath, err := filepath.Abs(path)
	if err != nil {
		return Result{}, err
	}

	realPath, err := filepath.EvalSymlinks(parsePath)
	if err != nil {
		return Result{}, err
	}

	homeDir, err := userHomeDir()
	if err != nil {
		return Result{}, err
	}

	state := loadState{
		trustedRoots: trustedRoots(realPath, homeDir),
		homeDir:      homeDir,
		stack:        make(map[string]struct{}),
	}

	file, err := loadFile(parsePath, filepath.Clean(path), state)
	if err != nil {
		return Result{}, err
	}

	hosts, err := collectHosts(file, state)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Path:  path,
		Hosts: hosts,
	}, nil
}

type loadedFile struct {
	parsePath   string
	realPath    string
	displayPath string
	config      *ssh.Config
}

type flattenedBlock struct {
	block *ssh.Host
}

type visibleHost struct {
	block      *ssh.Host
	sourcePath string
}

type loadState struct {
	trustedRoots []string
	homeDir      string
	stack        map[string]struct{}
}

func loadFile(parsePath, displayPath string, state loadState) (loadedFile, error) {
	realPath, err := filepath.EvalSymlinks(parsePath)
	if err != nil {
		return loadedFile{}, err
	}

	if !isTrustedPath(state.trustedRoots, realPath) {
		return loadedFile{}, fmt.Errorf("sshconfig: unsafe include path %q", parsePath)
	}

	if _, ok := state.stack[realPath]; ok {
		return loadedFile{}, fmt.Errorf("sshconfig: include cycle detected at %q", displayPath)
	}

	contents, err := readConfigFile(realPath)
	if err != nil {
		return loadedFile{}, err
	}

	config, err := ssh.DecodeBytes(rewriteIncludes(contents))
	if err != nil {
		return loadedFile{}, err
	}

	if err := validateConfig(config); err != nil {
		return loadedFile{}, err
	}

	return loadedFile{
		parsePath:   parsePath,
		realPath:    realPath,
		displayPath: displayPath,
		config:      config,
	}, nil
}

func collectHosts(file loadedFile, state loadState) ([]Host, error) {
	flattened, err := flattenConfig(file, state)
	if err != nil {
		return nil, err
	}

	visible, err := collectVisibleHosts(file, state)
	if err != nil {
		return nil, err
	}

	config := &ssh.Config{Hosts: make([]*ssh.Host, 0, len(flattened))}
	for _, block := range flattened {
		config.Hosts = append(config.Hosts, block.block)
	}

	out := make([]Host, 0, len(visible))
	for _, host := range visible {
		out = append(out, normalizeHost(config, host.block, host.sourcePath))
	}

	return out, nil
}

func collectVisibleHosts(file loadedFile, state loadState) ([]visibleHost, error) {
	state = state.clone()
	state.stack[file.realPath] = struct{}{}

	out := make([]visibleHost, 0)

	for i, block := range file.config.Hosts {
		for _, node := range block.Nodes {
			include, ok := node.(*ssh.KV)
			if !ok || !strings.EqualFold(include.Key, porticoIncludeKey) {
				continue
			}

			matches, err := includeMatches(file, include.Value, state)
			if err != nil {
				return nil, err
			}

			for _, match := range matches {
				child, err := loadFile(match.parsePath, match.displayPath, state)
				if err != nil {
					return nil, err
				}

				childHosts, err := collectVisibleHosts(child, state)
				if err != nil {
					return nil, err
				}

				out = append(out, childHosts...)
			}
		}

		if i == 0 {
			continue
		}

		out = append(out, visibleHost{block: block, sourcePath: file.displayPath})
	}

	return out, nil
}

func flattenConfig(file loadedFile, state loadState) ([]flattenedBlock, error) {
	state = state.clone()
	state.stack[file.realPath] = struct{}{}

	out := make([]flattenedBlock, 0)

	for _, block := range file.config.Hosts {
		segments, err := flattenBlock(file, block, state)
		if err != nil {
			return nil, err
		}

		out = append(out, segments...)
	}

	return out, nil
}

func flattenBlock(file loadedFile, block *ssh.Host, state loadState) ([]flattenedBlock, error) {
	out := make([]flattenedBlock, 0)
	segmentNodes := make([]ssh.Node, 0, len(block.Nodes))
	flushSegment := func() {
		out = append(out, flattenedBlock{block: cloneBlock(block, segmentNodes)})
		segmentNodes = nil
	}

	for _, node := range block.Nodes {
		include, ok := node.(*ssh.KV)
		if !ok || !strings.EqualFold(include.Key, porticoIncludeKey) {
			segmentNodes = append(segmentNodes, node)
			continue
		}

		if len(segmentNodes) > 0 {
			flushSegment()
		}

		matches, err := includeMatches(file, include.Value, state)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			child, err := loadFile(match.parsePath, match.displayPath, state)
			if err != nil {
				return nil, err
			}

			childBlocks, err := flattenConfig(child, state)
			if err != nil {
				return nil, err
			}

			out = append(out, childBlocks...)
		}
	}

	if len(segmentNodes) > 0 || len(out) == 0 {
		flushSegment()
	}

	return out, nil
}

func cloneBlock(block *ssh.Host, nodes []ssh.Node) *ssh.Host {
	patterns := append([]*ssh.Pattern(nil), block.Patterns...)
	clonedNodes := append([]ssh.Node(nil), nodes...)

	return &ssh.Host{
		Patterns: patterns,
		Nodes:    clonedNodes,
	}
}

func (s loadState) clone() loadState {
	stack := make(map[string]struct{}, len(s.stack)+1)
	for path := range s.stack {
		stack[path] = struct{}{}
	}

	s.stack = stack
	return s
}

func normalizeHost(config *ssh.Config, block *ssh.Host, sourcePath string) Host {
	patterns := make([]string, 0, len(block.Patterns))
	for _, pattern := range block.Patterns {
		patterns = append(patterns, pattern.String())
	}

	alias := strings.Join(patterns, " ")
	lookupAlias := hostLookupAlias(patterns)
	hostname := resolvedValue(config, lookupAlias, "HostName")
	if hostname == "" {
		hostname = alias
	}

	return Host{
		Alias:         alias,
		Patterns:      patterns,
		Hostname:      hostname,
		User:          resolvedValue(config, lookupAlias, "User"),
		Port:          resolvedValue(config, lookupAlias, "Port"),
		IdentityFiles: resolvedValues(config, lookupAlias, "IdentityFile"),
		ProxyJump:     resolvedValue(config, lookupAlias, "ProxyJump"),
		SourcePath:    sourcePath,
		Wildcard:      hasWildcard(patterns),
	}
}

func hostLookupAlias(patterns []string) string {
	for _, pattern := range patterns {
		if !strings.HasPrefix(pattern, "!") {
			return pattern
		}
	}

	if len(patterns) == 0 {
		return ""
	}

	return patterns[0]
}

func resolvedValue(config *ssh.Config, alias, key string) string {
	if alias == "" {
		return ""
	}

	value, err := config.Get(alias, key)
	if err != nil {
		return ""
	}

	return value
}

func resolvedValues(config *ssh.Config, alias, key string) []string {
	if alias == "" {
		return nil
	}

	values, err := config.GetAll(alias, key)
	if err != nil {
		return nil
	}

	return values
}

func firstValue(nodes []ssh.Node, key string) string {
	for _, node := range nodes {
		kv, ok := node.(*ssh.KV)
		if !ok || !strings.EqualFold(kv.Key, key) {
			continue
		}

		return kv.Value
	}

	return ""
}

func allValues(nodes []ssh.Node, key string) []string {
	var values []string

	for _, node := range nodes {
		kv, ok := node.(*ssh.KV)
		if !ok || !strings.EqualFold(kv.Key, key) {
			continue
		}

		values = append(values, kv.Value)
	}

	return values
}

func validateConfig(config *ssh.Config) error {
	for _, block := range config.Hosts {
		for _, node := range block.Nodes {
			kv, ok := node.(*ssh.KV)
			if !ok {
				continue
			}

			if strings.EqualFold(kv.Key, "HostName") && strings.TrimSpace(kv.Value) == "" {
				return fmt.Errorf("sshconfig: %s has empty value", kv.Key)
			}
		}
	}

	return nil
}

func hasWildcard(patterns []string) bool {
	for _, pattern := range patterns {
		if strings.ContainsAny(pattern, "*!?") {
			return true
		}
	}

	return false
}

type includeMatch struct {
	parsePath   string
	displayPath string
}

func includeMatches(file loadedFile, value string, state loadState) ([]includeMatch, error) {
	directives := strings.Fields(value)
	matches := make([]includeMatch, 0)
	seen := make(map[string]struct{})

	for _, directive := range directives {
		globPattern := includeGlob(file, directive, state.homeDir)
		globMatches, err := filepath.Glob(globPattern)
		if err != nil {
			return nil, err
		}

		for _, match := range globMatches {
			realPath, err := filepath.EvalSymlinks(match)
			if err != nil {
				return nil, err
			}

			if !isTrustedPath(state.trustedRoots, realPath) {
				return nil, fmt.Errorf("sshconfig: unsafe include path %q", directive)
			}

			if _, ok := seen[realPath]; ok {
				continue
			}

			seen[realPath] = struct{}{}
			matches = append(matches, includeMatch{
				parsePath:   match,
				displayPath: displayChildPath(file.displayPath, directive, match),
			})
		}
	}

	return matches, nil
}

func includeGlob(file loadedFile, directive, homeDir string) string {
	switch {
	case strings.HasPrefix(directive, "~/"):
		return filepath.Join(homeDir, directive[2:])
	case filepath.IsAbs(directive):
		return directive
	default:
		return filepath.Join(filepath.Dir(file.realPath), directive)
	}
}

func displayChildPath(parentDisplayPath, directive, realPath string) string {
	if filepath.IsAbs(parentDisplayPath) || filepath.IsAbs(directive) || strings.HasPrefix(directive, "~/") {
		return realPath
	}

	return filepath.Clean(filepath.Join(filepath.Dir(parentDisplayPath), directive))
}

func isWithinRoot(rootDir, path string) bool {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func userHomeDir() (string, error) {
	return os.UserHomeDir()
}

const porticoIncludeKey = "PorticoInclude"

const maxConfigFileSize = 1024 * 1024

var systemSSHRootDir = func() string {
	return filepath.Join(string(filepath.Separator), "etc", "ssh")
}

func trustedRoots(realPath, homeDir string) []string {
	standardRoots := standardSSHRoots(homeDir)
	homeSSHRoot := resolvedHomeSSHRoot(homeDir)
	homeRoot := resolvedHomeDir(homeDir)
	for _, root := range standardRoots {
		if isWithinRoot(root, realPath) {
			roots := append([]string(nil), standardRoots...)
			if homeSSHRoot != "" && root == homeSSHRoot && homeRoot != "" && !containsRoot(roots, homeRoot) {
				roots = append(roots, homeRoot)
			}
			return roots
		}
	}

	return []string{filepath.Dir(realPath)}
}

func standardSSHRoots(homeDir string) []string {
	roots := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	for _, candidate := range []string{filepath.Join(homeDir, ".ssh"), systemSSHRootDir()} {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}

		if _, ok := seen[resolved]; ok {
			continue
		}

		seen[resolved] = struct{}{}
		roots = append(roots, resolved)
	}

	return roots
}

func resolvedHomeSSHRoot(homeDir string) string {
	resolved, err := filepath.EvalSymlinks(filepath.Join(homeDir, ".ssh"))
	if err != nil {
		return ""
	}

	return resolved
}

func resolvedHomeDir(homeDir string) string {
	resolved, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return ""
	}

	return resolved
}

func containsRoot(roots []string, candidate string) bool {
	for _, root := range roots {
		if root == candidate {
			return true
		}
	}

	return false
}

func isTrustedPath(roots []string, path string) bool {
	for _, root := range roots {
		if isWithinRoot(root, path) {
			return true
		}
	}

	return false
}

func readConfigFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sshconfig: non-regular include path %q", path)
	}

	if info.Size() > maxConfigFileSize {
		return nil, fmt.Errorf("sshconfig: config file %q too large", path)
	}

	return os.ReadFile(path)
}

func rewriteIncludes(contents []byte) []byte {
	lines := strings.Split(string(contents), "\n")
	for i, line := range lines {
		beforeComment, comment, hasComment := strings.Cut(line, "#")
		fields := strings.Fields(beforeComment)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "Include") {
			continue
		}

		lines[i] = strings.Join([]string{porticoIncludeKey, strings.Join(fields[1:], " ")}, " ")
		if hasComment {
			lines[i] += "#" + comment
		}
	}

	return []byte(strings.Join(lines, "\n"))
}
