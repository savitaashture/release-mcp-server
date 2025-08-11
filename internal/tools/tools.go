package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func Add(_ context.Context, s *mcp.Server) error {
	// Register create-release-branches tool
	branchTool := &mcp.Tool{
		Name:        "create-release-branches",
		Description: "Creates release branches for Tekton components",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"minor_version": {
					Type:        "string",
					Description: "Minor version number (e.g., '1.19')",
				},
			},
			Required: []string{"minor_version"},
		},
	}

	branchHandler := func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
		// Extract parameters
		minorVersion, ok := params.Arguments["minor_version"].(string)
		if !ok || minorVersion == "" {
			return nil, fmt.Errorf("minor_version parameter is required")
		}

		if _, err := createBranch(minorVersion); err != nil {
			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create branches: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully created release branches for version %s", minorVersion)}},
		}, nil
	}

	s.AddTool(branchTool, branchHandler)

	// Register configure-hack-repo tool
	hackTool := &mcp.Tool{
		Name:        "configure-hack-repo",
		Description: "Configures the hack repository for a new release and creates a pull request",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"minor_version": {
					Type:        "string",
					Description: "Minor version number (e.g., '1.21')",
				},
				"ocp_version": {
					Type:        "string",
					Description: "OpenShift Container Platform version",
				},
				"upstream_versions": {
					Type: "object",
					AdditionalProperties: &jsonschema.Schema{
						Type:        "string",
						Description: "Upstream version for each component (e.g., '0.25.x' for chains)",
					},
					Description: "Map of component names to their upstream versions",
				},
			},
			Required: []string{"minor_version"},
		},
	}

	hackHandler := func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
		// Extract parameters
		minorVersion, ok := params.Arguments["minor_version"].(string)
		if !ok || minorVersion == "" {
			return nil, fmt.Errorf("minor_version parameter is required")
		}

		ocpVersion, _ := params.Arguments["ocp_version"].(string)

		// Extract upstream versions map
		upstreamVersions := make(map[string]string)
		if upstream, ok := params.Arguments["upstream_versions"].(map[string]interface{}); ok {
			for k, v := range upstream {
				if strVal, ok := v.(string); ok {
					upstreamVersions[k] = strVal
				}
			}
		}

		repoPath := filepath.Join(os.TempDir(), "hack-repo")

		config := HackConfig{
			MinorVersion:   minorVersion,
			OCPVersion:     ocpVersion,
			RepoPath:       repoPath,
			UpstreamConfig: upstreamVersions,
		}

		if err := ConfigureHackRepo(config); err != nil {
			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to configure hack repository: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: "Successfully configured hack repository and created pull request"}},
		}, nil
	}

	s.AddTool(hackTool, hackHandler)

	// Register create-release-plans tool
	releasePlanTool := &mcp.Tool{
		Name:        "create-release-plans",
		Description: "Creates ReleasePlanAdmission and ReleasePlan files for Tekton components",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"minor_version": {
					Type:        "string",
					Description: "Minor version number (e.g., '1.21')",
				},
				"patch_version": {
					Type:        "string",
					Description: "Optional patch version number",
				},
				"ocp_versions": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "string",
					},
					Description: "List of OCP versions (e.g., ['4-15', '4-16']). Defaults to ['4-15', '4-16', '4-17', '4-18', '4-19']",
				},
			},
			Required: []string{"minor_version"},
		},
	}

	releasePlanHandler := func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
		// Extract parameters
		minorVersion, ok := params.Arguments["minor_version"].(string)
		if !ok || minorVersion == "" {
			return nil, fmt.Errorf("minor_version parameter is required")
		}

		// Patch version is optional
		patchVersion, _ := params.Arguments["patch_version"].(string)

		// Get OCP versions from input or use defaults
		var ocpVersions []string
		if versions, ok := params.Arguments["ocp_versions"].([]interface{}); ok && len(versions) > 0 {
			for _, v := range versions {
				if strVal, ok := v.(string); ok {
					ocpVersions = append(ocpVersions, strVal)
				}
			}
		}
		if len(ocpVersions) == 0 {
			ocpVersions = []string{"4-15", "4-16", "4-17", "4-18", "4-19"}
		}

		// Define component configurations
		components := map[string][]ComponentConfig{
			"cli": {
				{Name: "tkn", Repository: "pipelines-cli-tkn-rhel9"},
			},
			"core": {
				{Name: "controller", Repository: "pipelines-core-controller-rhel9"},
				{Name: "webhook", Repository: "pipelines-core-webhook-rhel9"},
			},
			"operator": {
				{Name: "operator", Repository: "pipelines-rhel9-operator"},
				{Name: "proxy", Repository: "pipelines-operator-proxy-rhel9"},
				{Name: "webhook", Repository: "pipelines-operator-webhook-rhel9"},
			},
			"fbc": {}, // FBC has special handling
		}

		config := RPAConfig{
			MinorVersion: minorVersion,
			PatchVersion: patchVersion,
			RepoPath:     filepath.Join(os.TempDir(), "konflux-release-data"),
			Components:   components,
			Environments: []string{"stage", "prod"},
			OCPVersions:  ocpVersions,
		}

		if err := createReleasePlans(config); err != nil {
			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create release plans: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: "Successfully created ReleasePlan and ReleasePlanAdmission files"}},
		}, nil
	}

	s.AddTool(releasePlanTool, releasePlanHandler)
	return nil
}

func result(s string) *mcp.CallToolResultFor[string] {
	return &mcp.CallToolResultFor[string]{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}
