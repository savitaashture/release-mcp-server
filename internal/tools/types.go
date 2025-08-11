package tools

// Repository represents a Git repository configuration
type Repository struct {
	Name         string
	SourceBranch string
	Skip         bool
	RepoURL      string
}

// Config holds the configuration for branch creation
type Config struct {
	MinorVersion string
	WorkDir      string
	Repositories []Repository
}
