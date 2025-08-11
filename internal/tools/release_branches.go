package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func createBranch(minorVersion string) (bool, error) {
	if minorVersion == "" {
		return false, fmt.Errorf("minor version is required")
	}

	fmt.Printf("Creating branches for version %s\n", minorVersion)

	// Create a temporary working directory
	workDir, err := os.MkdirTemp("", "tekton-release-*")
	if err != nil {
		return false, fmt.Errorf("failed to create working directory: %w", err)
	}
	defer os.RemoveAll(workDir) // Clean up when done

	fmt.Println("workDir:", workDir)

	// Default repositories configuration
	config := Config{
		MinorVersion: minorVersion,
		WorkDir:      workDir,
		Repositories: []Repository{
			{
				Name:         "pipeline",
				SourceBranch: "next",
				RepoURL:      "git@github.com:openshift-pipelines/tektoncd-pipeline.git",
			},
			{
				Name:         "triggers",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-triggers.git",
			},
			{
				Name:         "chains",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-chains.git",
			},
			{
				Name:         "results",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-results.git",
			},
			{
				Name:         "cli",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-cli",
			},
			{
				Name:         "hub",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-hub",
			},
			{
				Name:         "pac",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/pac-downstream",
			},
			{
				Name:         "cache",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tekton-caches",
			},
			{
				Name:         "git-init",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/tektoncd-git-clone",
			},
			{
				Name:         "operator",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/operator.git",
			},
			{
				Name:         "hack",
				SourceBranch: "next",
				RepoURL:      "git@github.com/openshift-pipelines/hack.git",
			},
			// Skipped repositories
			{
				Name: "manual-approval-gate",
				Skip: true,
			},
			{
				Name: "opc",
				Skip: true,
			},
			{
				Name: "console-plugin",
				Skip: true,
			},
			{
				Name: "tektoncd-pruner",
				Skip: true,
			},
			{
				Name: "tekton-caches",
				Skip: true,
			},
		},
	}

	for _, repo := range config.Repositories {
		if repo.Skip {
			continue
		}

		if err := createBranchForRepo(repo, config); err != nil {
			return false, fmt.Errorf("failed to create branch for %s: %w", repo.Name, err)
		}
	}

	return true, nil
}

func createBranchForRepo(repo Repository, config Config) error {
	fmt.Println("Creating branch for repo:", repo.Name)

	// Create repository directory
	repoDir := filepath.Join(config.WorkDir, repo.Name)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", repo.Name, err)
	}
	fmt.Println("Repository directory:", repoDir)

	// Clone the repository
	fmt.Println("Cloning repository:", repo.RepoURL)
	cloneCmd := exec.Command("git", "clone", repo.RepoURL, ".")
	cloneCmd.Dir = repoDir
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository %s: %w", repo.Name, err)
	}

	// Fetch all branches
	fmt.Println("Fetching all branches")
	fetchCmd := exec.Command("git", "fetch", "--all")
	fetchCmd.Dir = repoDir
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch branches for %s: %w", repo.Name, err)
	}

	// Checkout source branch
	fmt.Printf("Checking out source branch: %s\n", repo.SourceBranch)
	checkoutCmd := exec.Command("git", "checkout", repo.SourceBranch)
	checkoutCmd.Dir = repoDir
	checkoutCmd.Stdout = os.Stdout
	checkoutCmd.Stderr = os.Stderr
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", repo.SourceBranch, err)
	}

	// Pull latest changes
	fmt.Println("Pulling latest changes")
	pullCmd := exec.Command("git", "pull", "origin", repo.SourceBranch)
	pullCmd.Dir = repoDir
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull latest changes for %s: %w", repo.Name, err)
	}

	// Create new branch
	newBranchName := fmt.Sprintf("release-v%s.x", config.MinorVersion)
	fmt.Printf("Creating new branch: %s\n", newBranchName)
	createBranchCmd := exec.Command("git", "checkout", "-b", newBranchName)
	createBranchCmd.Dir = repoDir
	createBranchCmd.Stdout = os.Stdout
	createBranchCmd.Stderr = os.Stderr
	if err := createBranchCmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", newBranchName, err)
	}

	// Push new branch to origin
	fmt.Printf("Pushing branch %s to origin\n", newBranchName)
	pushCmd := exec.Command("git", "push", "origin", newBranchName)
	pushCmd.Dir = repoDir
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w", newBranchName, err)
	}

	fmt.Printf("Successfully created and pushed branch %s for %s\n", newBranchName, repo.Name)
	return nil
}
