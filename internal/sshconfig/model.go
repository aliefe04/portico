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
	Wildcard      bool
}
