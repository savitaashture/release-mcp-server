# Release MCP Tools

This repository contains tools for managing Tekton releases through Model Context Protocol (MCP).

## Available Tools

### 1. Create Release Branches (`create-release-branches`)

This tool creates release branches for Tekton components.

**Input Parameters:**
- `minor_version`: The minor version to create branches for (e.g., "1.21")
- `patch_version`: The patch version to use (e.g., "0")
- `components`: List of component names to create branches for

**Functionality:**
- Clones each component's repository of openshift-pipelines
- Creates release branches (e.g., release-v1.21.x)
- Commits and pushes changes

### 2. Configure Hack Repository (`configure-hack-repo`)

This tool configures the [hack](https://github.com/openshift-pipelines/hack/) repository for a specific minor/patch version by updating component configurations.

**Input Parameters:**
- `minor_version`: The minor version to configure (e.g., "1.21")
- `upstream_versions`: Map of component names to their upstream versions

**Functionality:**
- Clones the hack repository
- Updates component configurations in YAML files
- Preserves existing YAML structure including patches
- Updates branches section for each component
- Creates and pushes changes to a new branch

### 3. Create Release Plans (`create-release-plans`)

This tool generates release plans and release plan admissions for components.

**Input Parameters:**
- `minor_version`: The minor version to create release plans for (e.g., "1.21")

**Functionality:**
- Clones the Konflux release data repository
- Generates ReleasePlanAdmission (RPA) files
- Generates ReleasePlan (RP) files
- Updates Kustomization files
- Runs build manifests script
- Creates and pushes changes to a new branch

## Environment Variables

The tools require certain environment variables to be set:

- `GITLAB_USERNAME`: GitLab username for authentication
- `GITLAB_TOKEN`: GitLab personal access token for authentication

## Usage Examples with NL

1. Create Release Branches:
```bash

# Call the create-release-branches tool
create release branches for 1.21 version
```

2. Configure Hack Repository:
```bash
# Set required environment variables
export GITLAB_USERNAME="your-username"
export GITLAB_TOKEN="your-token"

configure hack repo for 1.21 minorversion for the component name tektoncd-chains with upstream version release-v0.24.x, tektoncd-git-clone with upstream version release-v1.0.x, operator with upstream version release-v0.76.x, pac-downstream with upstream version release-v0.35.x, tektoncd-cli with upstream version release-v0.40.0, tektoncd-hub with upstream version release-v1.20.0, tektoncd-results with upstream version release-v0.14.x, tektoncd-triggers with upstream version release-v0.31.x, tektoncd-pipeline with upstream version release-v1.0.x, manual-approval-gate with version release-v0.5.0, tekton-caches with version release-v0.1.x, tektoncd-pruner with version release-v0.2.x
```

3. Create Release Plans:
```bash
# Set required environment variables
export GITLAB_USERNAME="your-username"
export GITLAB_TOKEN="your-token"

# Call the create-release-plans tool
create release plan for 1.21
```

## Repository Structure

```
release-mcp/
├── cmd/
│   └── release-mcp-server/     # MCP server implementation
├── internal/
│   └── tools/                  # Tool implementations
│       ├── hack_config.go      # Hack repository configuration
│       ├── hack_config.go      # Configure hack repository
│       ├── release_plan.go     # Release files generation
│       ├── release_branches.go # creation of branches on each repository
│       └── tools.go            # Tool registration
└── README.md                   # Documentation
```