package sshconfig

type Result struct {
	Path     string
	Hosts    []Host
	Warnings []string
}

type Host struct {
	Alias         string
	Patterns      []string
	Hostname      string
	User          string
	Port          string
	IdentityFiles []string
	ProxyJump     string
	SourcePath    string
	filePath      string
	Wildcard      bool
}

func (h Host) FilePath() string {
	return h.filePath
}
