package tools

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// HackConfig represents the configuration for hack repository updates
type HackConfig struct {
	MinorVersion   string
	OCPVersion     string
	RepoPath       string
	UpstreamConfig map[string]string // map of component name to upstream version
}

// RepoConfig represents the repository configuration in YAML
type RepoConfig struct {
	Name       string      `yaml:"name"`
	Upstream   string      `yaml:"upstream,omitempty"`
	Components []Component `yaml:"components"`
	Patches    []Patch     `yaml:"patches,omitempty"`
	Tekton     *TektonInfo `yaml:"tekton,omitempty"`
	Branches   []Branch    `yaml:"branches"`
}

// Component represents a component configuration
type Component struct {
	Name          string `yaml:"name"`
	PrefetchInput string `yaml:"prefetch-input,omitempty"`
}

type Patch struct {
	Name   string `yaml:"name"`
	Script string `yaml:"script,omitempty"`
}

// TektonInfo represents Tekton-specific configuration
type TektonInfo struct {
	WatchedSources string `yaml:"watched-sources"`
}

// Branch represents branch configuration
type Branch struct {
	Name     string   `yaml:"name"`
	Upstream string   `yaml:"upstream,omitempty"`
	Patches  []Patch  `yaml:"patches,omitempty"`
	Versions []string `yaml:"versions"`
}

// BranchConfig represents a branch configuration
type BranchConfig struct {
	Name     string
	Upstream string
	Patches  []Patch
	Versions []string
}

// ComponentMapping maps repository names to their component names
var componentMapping = map[string]string{
	"tektoncd-pipeline":    "pipeline",
	"tektoncd-chains":      "chains",
	"tektoncd-git-clone":   "git-init",
	"operator":             "operator",
	"pac-downstream":       "pac",
	"tektoncd-cli":         "cli",
	"tektoncd-hub":         "hub",
	"tektoncd-results":     "results",
	"tektoncd-triggers":    "triggers",
	"manual-approval-gate": "manual-approval-gate",
	"tekton-caches":        "cache",
	"tektoncd-pruner":      "pruner",
}

// Special components that use version as branch name
var specialComponents = map[string]bool{
	"manual-approval-gate": true,
	"cache":                true,
	"pruner":               true,
}

func createBranchConfig(minorVersion string, repoName string, hasUpstream bool, upstreamVersions map[string]string) BranchConfig {
	componentName, ok := componentMapping[repoName]
	if !ok {
		// Default configuration for unknown components
		return BranchConfig{
			Name:     fmt.Sprintf("release-v%s.x", minorVersion),
			Versions: []string{minorVersion},
		}
	}

	// Check if this is a special component that uses version as name
	if specialComponents[componentName] {
		if version, ok := upstreamVersions[componentName]; ok {
			return BranchConfig{
				Name:     version, // Use version directly as name for special components
				Versions: []string{minorVersion},
			}
		}
	}

	// Regular component configuration
	branchConfig := BranchConfig{
		Name:     fmt.Sprintf("release-v%s.x", minorVersion),
		Versions: []string{minorVersion},
	}

	// Set upstream version if component has upstream
	if hasUpstream {
		if upstreamVersion, ok := upstreamVersions[componentName]; ok {
			branchConfig.Upstream = upstreamVersion
		}
	}

	return branchConfig
}

func formatBranchYAML(branchConfig BranchConfig, indentation string, hasPatches bool) []string {
	var lines []string
	lines = append(lines, indentation+"- name: "+branchConfig.Name)
	if branchConfig.Upstream != "" {
		lines = append(lines, indentation+"  upstream: "+branchConfig.Upstream)
	}
	if hasPatches {
		lines = append(lines, indentation+"  patches: *patches")
	}
	lines = append(lines, indentation+"  versions:")
	for _, version := range branchConfig.Versions {
		lines = append(lines, fmt.Sprintf("%s    - \"%s\"", indentation, version))
	}
	return lines
}

func ConfigureHackRepo(config HackConfig) error {
	// Clone hack repository
	if err := cloneHackRepo(config); err != nil {
		return fmt.Errorf("failed to clone hack repository: %w", err)
	}

	// Create a new branch for changes
	if err := createPRBranch(config); err != nil {
		return fmt.Errorf("failed to create PR branch: %w", err)
	}

	// Update Konflux configurations
	if err := updateKonfluxConfigs(config); err != nil {
		return fmt.Errorf("failed to update Konflux configurations: %w", err)
	}

	// Update repository branch configurations
	if err := updateRepoBranches(config); err != nil {
		return fmt.Errorf("failed to update repository branch configurations: %w", err)
	}

	// Create and push pull request
	prURL, err := createAndPushPR(config)
	if err != nil {
		return fmt.Errorf("failed to create and push PR: %w", err)
	}

	fmt.Printf("\nPull Request created successfully: %s\n", prURL)
	return nil
}

func cloneHackRepo(config HackConfig) error {
	branchName := fmt.Sprintf("release-v%s.x", config.MinorVersion)
	cloneCmd := exec.Command("git", "clone",
		"git@github.com:openshift-pipelines/hack.git",
		"-b", branchName,
		config.RepoPath)

	fmt.Println("Cloning hack repository...with branch", branchName)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	return nil
}

func createPRBranch(config HackConfig) error {
	// Create a new branch for our changes
	branchName := fmt.Sprintf("update-konflux-config-%s", time.Now().Format("20060102150405"))
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = config.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create PR branch: %w", err)
	}
	return nil
}

func createAndPushPR(config HackConfig) (string, error) {
	// Stage all changes
	stageCmd := exec.Command("git", "add", ".")
	stageCmd.Dir = config.RepoPath
	stageCmd.Stdout = os.Stdout
	stageCmd.Stderr = os.Stderr
	if err := stageCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to stage changes: %w", err)
	}

	// Create commit
	commitMsg := fmt.Sprintf("Update Konflux configuration for release v%s", config.MinorVersion)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = config.RepoPath
	commitCmd.Stdout = os.Stdout
	commitCmd.Stderr = os.Stderr
	if err := commitCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to commit changes: %w", err)
	}

	// Get current branch name
	currentBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchCmd.Dir = config.RepoPath
	branchOutput, err := currentBranchCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(branchOutput))

	// Push to your fork
	pushCmd := exec.Command("git", "push", "-f", "origin", currentBranch)
	pushCmd.Dir = config.RepoPath
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to push changes: %w", err)
	}

	// Get fork owner from git config
	ownerCmd := exec.Command("git", "config", "--get", "remote.origin.url")
	ownerCmd.Dir = config.RepoPath
	ownerOutput, err := ownerCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	remoteURL := strings.TrimSpace(string(ownerOutput))

	// Extract owner from URL (handles both SSH and HTTPS URLs)
	var owner string
	if strings.HasPrefix(remoteURL, "git@") {
		// SSH URL: git@github.com:owner/repo.git
		parts := strings.Split(remoteURL, ":")
		if len(parts) > 1 {
			owner = strings.Split(parts[1], "/")[0]
		}
	} else {
		// HTTPS URL: https://github.com/owner/repo.git
		parts := strings.Split(remoteURL, "/")
		for i, part := range parts {
			if part == "github.com" && i+1 < len(parts) {
				owner = parts[i+1]
				break
			}
		}
	}

	if owner == "" {
		return "", fmt.Errorf("could not determine fork owner from URL: %s", remoteURL)
	}

	// Create PR using gh CLI
	prTitle := fmt.Sprintf("Update Konflux configuration for release v%s", config.MinorVersion)

	// Build PR body
	var ocpNote string
	if config.OCPVersion != "" {
		ocpNote = fmt.Sprintf("- Added new OCP %s configuration", config.OCPVersion)
	}

	prBody := fmt.Sprintf(`Update Konflux configuration for release v%s

Changes:
- Updated version references for release v%s
- Updated branch configurations in repos directory
%s`,
		config.MinorVersion,
		config.MinorVersion,
		ocpNote,
	)

	// Create PR and capture output
	var stdout, stderr bytes.Buffer
	prCmd := exec.Command("gh", "pr", "create",
		"--title", prTitle,
		"--body", prBody,
		"--repo", "openshift-pipelines/hack",
		"--head", fmt.Sprintf("%s:%s", owner, currentBranch),
		"--base", fmt.Sprintf("release-v%s.x", config.MinorVersion))
	prCmd.Dir = config.RepoPath
	prCmd.Stdout = &stdout
	prCmd.Stderr = &stderr
	if err := prCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create pull request: %v\nError details: %s", err, stderr.String())
	}

	// The output from gh pr create is the PR URL
	prURL := strings.TrimSpace(stdout.String())
	return prURL, nil
}

func updateKonfluxConfigs(config HackConfig) error {
	konfluxDir := filepath.Join(config.RepoPath, "config", "konflux")

	// Read all files in the konflux directory
	entries, err := os.ReadDir(konfluxDir)
	if err != nil {
		return fmt.Errorf("failed to read konflux directory: %w", err)
	}

	// Update version in each file
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			filePath := filepath.Join(konfluxDir, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", entry.Name(), err)
			}

			// Replace "next" with the release version
			newContent := strings.ReplaceAll(string(content), "next", config.MinorVersion)

			if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", entry.Name(), err)
			}
		}
	}

	// Create new OCP version file if needed
	if config.OCPVersion != "" {
		newFilePath := filepath.Join(konfluxDir, fmt.Sprintf("openshift-pipelines-index-%s.yaml", config.OCPVersion))
		// TODO: Implement template-based file creation for new OCP version
		// For now, just print a message
		fmt.Printf("Note: To add support for OCP %s, create a new file: %s\n", config.OCPVersion, newFilePath)
		fmt.Printf("Follow the reference example from an existing openshift-pipelines-index-*.yaml file\n")
	}

	return nil
}

func updateRepoBranches(config HackConfig) error {
	reposDir := filepath.Join(config.RepoPath, "config", "konflux", "repos")

	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return fmt.Errorf("failed to read repos directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			filePath := filepath.Join(reposDir, entry.Name())

			// Read the original content as string to preserve exact format
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", entry.Name(), err)
			}

			// Parse YAML to find the repository name and upstream status
			var yamlData map[string]interface{}
			if err := yaml.Unmarshal(content, &yamlData); err != nil {
				return fmt.Errorf("failed to parse YAML: %w", err)
			}

			repoName := yamlData["name"].(string)
			hasUpstream := yamlData["upstream"] != nil
			hasPatches := yamlData["patches"] != nil

			// Create branch config
			branchConfig := createBranchConfig(config.MinorVersion, repoName, hasUpstream, config.UpstreamConfig)

			// Format branch YAML
			branchLines := formatBranchYAML(branchConfig, "  ", hasPatches)
			branchYAML := strings.Join(branchLines, "\n")

			// Find the start of the branches section
			branchesStart := strings.Index(string(content), "\nbranches:")
			if branchesStart == -1 {
				// If no branches section exists, add it at the end
				newContent := string(content)
				if !strings.HasSuffix(newContent, "\n") {
					newContent += "\n"
				}
				newContent += "branches:\n" + branchYAML
				if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
			} else {
				// Find the end of the branches section
				contentAfterBranches := string(content)[branchesStart+1:]
				nextSection := strings.Index(contentAfterBranches, "\n\n")
				if nextSection == -1 {
					nextSection = len(contentAfterBranches)
				}

				// Replace only the branches section
				newContent := string(content)[:branchesStart+1] + "branches:\n" + branchYAML + string(content)[branchesStart+nextSection+1:]
				if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
			}

			fmt.Printf("Updated %s with version %s\n", repoName, config.MinorVersion)
		}
	}

	return nil
}
