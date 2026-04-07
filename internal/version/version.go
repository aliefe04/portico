package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Summary() string {
	return Version
}
