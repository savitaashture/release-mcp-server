package tools

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// ComponentConfig represents a component's configuration
type ComponentConfig struct {
	Name       string
	Repository string
}

// RPAConfig represents the configuration for ReleasePlanAdmission and ReleasePlan creation
type RPAConfig struct {
	MinorVersion string
	PatchVersion string // Optional, if not provided it's a minor release
	RepoPath     string
	Components   map[string][]ComponentConfig
	Environments []string
	OCPVersions  []string // List of OCP versions for FBC
}

// getRegistryURL returns the appropriate registry URL based on environment
func getRegistryURL(env string) string {
	if env == "stage" {
		return "registry.stage.redhat.io"
	}
	return "registry.redhat.io"
}

// getFBCConfig returns environment-specific FBC configuration
func getFBCConfig(env string) map[string]interface{} {
	if env == "stage" {
		return map[string]interface{}{
			"stagedIndex":           true,
			"fromIndex":             "registry-proxy.engineering.redhat.com/rh-osbs/iib-pub-pending:{{ OCP_VERSION }}",
			"targetIndex":           "",
			"publishingCredentials": "staged-index-fbc-publishing-credentials",
			"requestTimeoutSeconds": 1500,
			"buildTimeoutSeconds":   1500,
			"allowedPackages":       []string{"openshift-pipelines-operator-rh"},
		}
	}
	return map[string]interface{}{
		"fromIndex":             "registry-proxy.engineering.redhat.com/rh-osbs/iib-pub:{{ OCP_VERSION }}",
		"targetIndex":           "quay.io/redhat-prod/redhat----redhat-operator-index:{{ OCP_VERSION }}",
		"publishingCredentials": "fbc-production-publishing-credentials-redhat-prod",
		"requestTimeoutSeconds": 1500,
		"buildTimeoutSeconds":   1500,
		"allowedPackages":       []string{"openshift-pipelines-operator-rh"},
	}
}

// getEnvSpecificValues returns environment-specific values
func getEnvSpecificValues(env string, isFBC bool) struct {
	Policy         string
	Intention      string
	ServiceAccount string
	RegistryURL    string
	BusinessUnit   string
} {
	if isFBC {
		if env == "stage" {
			return struct {
				Policy         string
				Intention      string
				ServiceAccount string
				RegistryURL    string
				BusinessUnit   string
			}{
				Policy:         "fbc-tekton-ecosystem-stage",
				Intention:      "staging",
				ServiceAccount: "release-index-image-staging",
				RegistryURL:    "registry.stage.redhat.io",
				BusinessUnit:   "hybrid-platforms",
			}
		}
		return struct {
			Policy         string
			Intention      string
			ServiceAccount string
			RegistryURL    string
			BusinessUnit   string
		}{
			Policy:         "fbc-tekton-ecosystem-prod",
			Intention:      "production",
			ServiceAccount: "release-index-image-prod",
			RegistryURL:    "registry.redhat.io",
			BusinessUnit:   "hybrid-platforms",
		}
	}
	// Non-FBC values
	if env == "stage" {
		return struct {
			Policy         string
			Intention      string
			ServiceAccount string
			RegistryURL    string
			BusinessUnit   string
		}{
			Policy:         "registry-standard-stage",
			Intention:      "staging",
			ServiceAccount: "release-registry-staging",
			RegistryURL:    "registry.stage.redhat.io",
			BusinessUnit:   "application-developer",
		}
	}
	return struct {
		Policy         string
		Intention      string
		ServiceAccount string
		RegistryURL    string
		BusinessUnit   string
	}{
		Policy:         "registry-standard",
		Intention:      "production",
		ServiceAccount: "release-registry-prod",
		RegistryURL:    "registry.redhat.io",
		BusinessUnit:   "application-developer",
	}
}

// RPATemplate represents the template for ReleasePlanAdmission
const RPATemplate = `apiVersion: appstudio.redhat.com/v1alpha1
kind: ReleasePlanAdmission
metadata:
  labels:
    release.appstudio.openshift.io/block-releases: "false"
    pp.engineering.redhat.com/business-unit: {{.EnvConfig.BusinessUnit}}
  name: {{if .IsFBC}}openshift-pipelines-{{.MinorVersion}}-fbc-{{.Env}}{{else}}openshift-pipelines-{{.Component}}-{{.MinorVersion}}-{{.Env}}{{end}}
  namespace: rhtap-releng-tenant
  annotations:
    rhel_target: el9
spec:
{{- if .IsFBC}}
  applications:
{{- range .OCPVersions}}
    - openshift-pipelines-index-{{.}}-{{$.MinorVersion}}
{{- end}}
{{- else}}
  applications: [ openshift-pipelines-{{.Component}}-{{.MinorVersion}} ]
{{- end}}
  origin: tekton-ecosystem-tenant
  policy: {{.EnvConfig.Policy}}
  data:
    releaseNotes:
      product_id: [ 604 ]
      product_name: "Red Hat OpenShift Pipelines"
      product_version: {{if .IsFBC}}fbc{{else}}{{.FullVersion}}{{end}}
{{- if .IsFBC}}
      references:
        - "https://docs.redhat.com/en/documentation/red_hat_openshift_pipelines/"
{{- end}}
      type: "{{.ReleaseType}}"
{{- if .IsFBC}}
    fbc:
{{- range $key, $value := .FBCConfig}}
{{- if eq $key "allowedPackages"}}
      allowedPackages:
{{- range $value}}
        - {{.}}
{{- end}}
{{- else}}
      {{$key}}: {{$value}}
{{- end}}
{{- end}}
{{- else}}
    mapping:
      components:
{{- range .SubComponents }}
        - name: tektoncd-{{$.Component}}-{{$.MinorVersion}}-{{.Name}}
          repository: "{{$.EnvConfig.RegistryURL}}/openshift-pipelines/{{.Repository}}"
          pushSourceContainer: true
{{- end }}
      defaults:
        tags:
          - "{{ "{{" }} git_sha {{ "}}" }}"
          - "{{ "{{" }} git_short_sha {{ "}}" }}"
          - "v{{.FullVersion}}"
          - "v{{.FullVersion}}-{{ "{{" }} timestamp {{ "}}" }}"
{{- end}}
    intention: {{.EnvConfig.Intention}}
  pipeline:
    serviceAccountName: {{.EnvConfig.ServiceAccount}}
    timeouts:
      pipeline: "10h0m0s"
      tasks: 10h0m0s
    pipelineRef:
      resolver: git
      params:
        - name: url
          value: "https://github.com/konflux-ci/release-service-catalog.git"
        - name: revision
          value: production
        - name: pathInRepo
{{- if .IsFBC}}
          value: "pipelines/managed/fbc-release/fbc-release.yaml"
{{- else}}
          value: "pipelines/managed/rh-advisories/rh-advisories.yaml"
{{- end}}`

// RPTemplate represents the template for ReleasePlan
const RPTemplate = `apiVersion: appstudio.redhat.com/v1alpha1
kind: ReleasePlan
metadata:
  labels:
    release.appstudio.openshift.io/auto-release: "false"
    release.appstudio.openshift.io/standing-attribution: "true"
    release.appstudio.openshift.io/releasePlanAdmission: openshift-pipelines-{{.Component}}-{{.MinorVersion}}-{{.Env}}
  name: openshift-pipelines-{{.Component}}-{{.MinorVersion}}-{{.Env}}-release-as-op
spec:
  application: openshift-pipelines-{{.Component}}-{{.MinorVersion}}
  target: rhtap-releng-tenant
  data:
    releaseNotes:
      references:
        - "https://docs.redhat.com/en/documentation/red_hat_openshift_pipelines"
      type: "{{.ReleaseType}}"
      solution: |
        Red Hat OpenShift Pipelines is a cloud-native, continuous integration and
        continuous delivery (CI/CD) solution based on Kubernetes resources.
        It uses Tekton building blocks to automate deployments across multiple
        platforms by abstracting away the underlying implementation details.
        Tekton introduces a number of standard custom resource definitions (CRDs)
        for defining CI/CD pipelines that are portable across Kubernetes distributions.
      description: "The {{.FullVersion}} release of Red Hat OpenShift Pipelines {{.Component | title}}."
      topic: |
        The {{.FullVersion}} GA release of Red Hat OpenShift Pipelines {{.Component | title}}..
        For more details see [product documentation](https://docs.redhat.com/en/documentation/red_hat_openshift_pipelines).
      synopsis: "Red Hat OpenShift Pipelines Release {{.FullVersion}}"
`

// titleCase converts a string to title case
func titleCase(s string) string {
	switch s {
	case "cli":
		return "CLI"
	case "fbc":
		return "FBC"
	default:
		return strings.Title(s)
	}
}

func createReleasePlans(config RPAConfig) error {
	fmt.Printf("DEBUG: Starting createReleasePlans with config: %+v\n", config)

	// Clone the konflux-release-data repository
	if err := cloneKonfluxRepo(config); err != nil {
		return fmt.Errorf("failed to clone konflux-release-data repository: %w", err)
	}
	fmt.Println("DEBUG: Successfully cloned konflux repo")

	// Create a new branch for changes
	branchName := fmt.Sprintf("add-release-plans-%s", config.MinorVersion)
	if err := createBranchInRepo(config.RepoPath, branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Create ReleasePlanAdmissions
	if err := createRPAs(config); err != nil {
		return fmt.Errorf("failed to create ReleasePlanAdmissions: %w", err)
	}
	fmt.Println("DEBUG: Successfully created ReleasePlanAdmissions in konflux repo")

	// Create ReleasePlans
	if err := createRPs(config); err != nil {
		return fmt.Errorf("failed to create ReleasePlans: %w", err)
	}
	fmt.Println("DEBUG: Successfully created ReleasePlans in konflux repo")

	// Update kustomization.yaml
	if err := updateKustomization(config); err != nil {
		return fmt.Errorf("failed to update kustomization.yaml: %w", err)
	}
	fmt.Println("DEBUG: Successfully updated kustomization.yaml in konflux repo")

	// Run build-manifests.sh
	if err := runBuildManifests(config); err != nil {
		return fmt.Errorf("failed to run build-manifests.sh: %w", err)
	}
	fmt.Println("DEBUG: Successfully ran build-manifests.sh")

	// Create and push merge request
	if err := createAndPushMR(config); err != nil {
		return fmt.Errorf("failed to create and push merge request: %w", err)
	}
	fmt.Println("DEBUG: Successfully created and pushed merge request in konflux repo")

	return nil
}

func cloneKonfluxRepo(config RPAConfig) error {
	username := os.Getenv("GITLAB_USERNAME")
	token := os.Getenv("GITLAB_TOKEN")
	fmt.Printf("DEBUG: Attempting to clone with username: %s (token length: %d)\n", username, len(token))
	if username == "" || token == "" {
		return fmt.Errorf("GITLAB_USERNAME and GITLAB_TOKEN environment variables must be set")
	}

	repoURL := fmt.Sprintf("https://%s:%s@gitlab.cee.redhat.com/sashture/konflux-release-data.git", username, token)
	fmt.Printf("DEBUG: Using repo URL: %s\n", strings.Replace(repoURL, token, "[REDACTED]", 1))

	cloneCmd := exec.Command("git", "clone", repoURL, config.RepoPath)
	fmt.Println("DEBUG: Executing git clone command...")

	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cloneCmd.Stdout = &stdout
	cloneCmd.Stderr = &stderr

	if err := cloneCmd.Run(); err != nil {
		fmt.Printf("DEBUG: Clone failed with error: %v\n", err)
		fmt.Printf("DEBUG: Clone stdout: %s\n", stdout.String())
		fmt.Printf("DEBUG: Clone stderr: %s\n", stderr.String())
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	fmt.Println("DEBUG: Clone command completed successfully")
	return nil
}

func createBranchInRepo(repoPath, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	return nil
}

func createRPAs(config RPAConfig) error {
	rpaBasePath := filepath.Join(config.RepoPath, "config", "kflux-prd-rh02.0fk9.p1", "product", "ReleasePlanAdmission", "tekton-ecosystem")

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(rpaBasePath, 0755); err != nil {
		return fmt.Errorf("failed to create RPA directory: %w", err)
	}

	tmpl, err := template.New("rpa").Parse(RPATemplate)
	if err != nil {
		return fmt.Errorf("failed to parse RPA template: %w", err)
	}

	// Get release type and full version
	releaseType, fullVersion := getReleaseType(config.MinorVersion, config.PatchVersion)

	// Create RPAs for each component and environment
	for componentName, subComponents := range config.Components {
		isFBC := componentName == "fbc"

		for _, env := range config.Environments {
			envConfig := getEnvSpecificValues(env, isFBC)

			data := struct {
				Component    string
				MinorVersion string
				FullVersion  string
				ReleaseType  string
				Env          string
				EnvConfig    struct {
					Policy         string
					Intention      string
					ServiceAccount string
					RegistryURL    string
					BusinessUnit   string
				}
				IsFBC         bool
				FBCConfig     map[string]interface{}
				OCPVersions   []string
				SubComponents []ComponentConfig
			}{
				Component:     componentName,
				MinorVersion:  config.MinorVersion,
				FullVersion:   fullVersion,
				ReleaseType:   releaseType,
				Env:           env,
				EnvConfig:     envConfig,
				IsFBC:         isFBC,
				FBCConfig:     getFBCConfig(env),
				OCPVersions:   config.OCPVersions,
				SubComponents: subComponents,
			}

			var fileName string
			if isFBC {
				fileName = fmt.Sprintf("openshift-pipelines-%s-fbc-%s.yaml", config.MinorVersion, env)
			} else {
				fileName = fmt.Sprintf("openshift-pipelines-%s-%s-%s.yaml", componentName, config.MinorVersion, env)
			}
			filePath := filepath.Join(rpaBasePath, fileName)

			file, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create RPA file %s: %w", fileName, err)
			}

			if err := tmpl.Execute(file, data); err != nil {
				file.Close()
				return fmt.Errorf("failed to write RPA template to %s: %w", fileName, err)
			}
			file.Close()
		}
	}

	return nil
}

func createRPs(config RPAConfig) error {
	rpBasePath := filepath.Join(config.RepoPath, "tenants-config", "cluster", "kflux-prd-rh02", "tenants", "tekton-ecosystem-tenant")

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(rpBasePath, 0755); err != nil {
		return fmt.Errorf("failed to create RP directory: %w", err)
	}

	// Create template with custom function
	tmpl := template.New("rp").Funcs(template.FuncMap{
		"title": titleCase,
	})

	tmpl, err := tmpl.Parse(RPTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse RP template: %w", err)
	}

	// Get release type and full version
	releaseType, fullVersion := getReleaseType(config.MinorVersion, config.PatchVersion)

	// Create RPs for each component and environment
	for componentName := range config.Components {
		for _, env := range config.Environments {
			data := struct {
				Component    string
				MinorVersion string
				FullVersion  string
				ReleaseType  string
				Env          string
			}{
				Component:    componentName,
				MinorVersion: config.MinorVersion,
				FullVersion:  fullVersion,
				ReleaseType:  releaseType,
				Env:          env,
			}

			fileName := fmt.Sprintf("openshift-pipelines-%s-%s-%s-release-as-op.yaml", componentName, config.MinorVersion, env)
			filePath := filepath.Join(rpBasePath, fileName)

			file, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create RP file %s: %w", fileName, err)
			}

			if err := tmpl.Execute(file, data); err != nil {
				file.Close()
				return fmt.Errorf("failed to write RP template to %s: %w", fileName, err)
			}
			file.Close()
		}
	}

	return nil
}

func updateKustomization(config RPAConfig) error {
	kustomizationPath := filepath.Join(config.RepoPath, "tenants-config", "cluster", "kflux-prd-rh02", "tenants", "tekton-ecosystem-tenant", "kustomization.yaml")

	// Read existing content
	content, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return fmt.Errorf("failed to read kustomization.yaml: %w", err)
	}

	// Add new resources
	var newResources []string
	for componentName := range config.Components {
		for _, env := range config.Environments {
			newResources = append(newResources,
				fmt.Sprintf("  - openshift-pipelines-%s-%s-%s-release-as-op.yaml",
					componentName, config.MinorVersion, env))
		}
	}

	// Update content
	lines := strings.Split(string(content), "\n")
	var updated []string
	resourcesFound := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "resources:" {
			resourcesFound = true
			updated = append(updated, line)
			updated = append(updated, newResources...)
		} else {
			updated = append(updated, line)
		}
	}

	if !resourcesFound {
		updated = append(updated, "resources:", "")
		updated = append(updated, newResources...)
	}

	// Write back to file
	if err := os.WriteFile(kustomizationPath, []byte(strings.Join(updated, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write kustomization.yaml: %w", err)
	}

	return nil
}

func runBuildManifests(config RPAConfig) error {
	fmt.Printf("DEBUG: Changing directory to: %s\n", config.RepoPath)
	if err := os.Chdir(config.RepoPath); err != nil {
		fmt.Printf("DEBUG: Failed to change directory: %v\n", err)
		return fmt.Errorf("failed to change directory to %s: %w", config.RepoPath, err)
	}

	scriptPath := filepath.Join("tenants-config", "build-manifests.sh")
	fmt.Printf("DEBUG: Attempting to run build-manifests.sh from path: %s\n", scriptPath)

	cmd := exec.Command("./" + scriptPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("DEBUG: build-manifests.sh failed with error: %v\n", err)
		fmt.Printf("DEBUG: build-manifests.sh stdout: %s\n", stdout.String())
		fmt.Printf("DEBUG: build-manifests.sh stderr: %s\n", stderr.String())
		return fmt.Errorf("failed to run build-manifests.sh: %w", err)
	}
	fmt.Printf("DEBUG: Successfully ran build-manifests.sh\n")
	return nil
}

func createAndPushMR(config RPAConfig) error {
	fmt.Println("DEBUG: Starting createAndPushMR function")

	// Stage all changes
	fmt.Printf("DEBUG: Staging changes in directory: %s\n", config.RepoPath)
	stageCmd := exec.Command("git", "add", ".")
	stageCmd.Dir = config.RepoPath
	var stageStdout, stageStderr bytes.Buffer
	stageCmd.Stdout = &stageStdout
	stageCmd.Stderr = &stageStderr
	if err := stageCmd.Run(); err != nil {
		fmt.Printf("DEBUG: Failed to stage changes. Error: %v\n", err)
		fmt.Printf("DEBUG: git add stdout: %s\n", stageStdout.String())
		fmt.Printf("DEBUG: git add stderr: %s\n", stageStderr.String())
		return fmt.Errorf("failed to stage changes: %w", err)
	}
	fmt.Println("DEBUG: Successfully staged changes")

	// Create commit
	commitMsg := fmt.Sprintf("Add ReleasePlan and ReleasePlanAdmission for v%s", config.MinorVersion)
	fmt.Printf("DEBUG: Creating commit with message: %s\n", commitMsg)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = config.RepoPath
	var commitStdout, commitStderr bytes.Buffer
	commitCmd.Stdout = &commitStdout
	commitCmd.Stderr = &commitStderr
	if err := commitCmd.Run(); err != nil {
		fmt.Printf("DEBUG: Failed to create commit. Error: %v\n", err)
		fmt.Printf("DEBUG: git commit stdout: %s\n", commitStdout.String())
		fmt.Printf("DEBUG: git commit stderr: %s\n", commitStderr.String())
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	fmt.Println("DEBUG: Successfully created commit")

	// Create and checkout new branch
	branchName := fmt.Sprintf("release-plan-v%s", config.MinorVersion)
	fmt.Printf("DEBUG: Creating and checking out branch: %s\n", branchName)
	checkoutCmd := exec.Command("git", "checkout", "-b", branchName)
	checkoutCmd.Dir = config.RepoPath
	var checkoutStdout, checkoutStderr bytes.Buffer
	checkoutCmd.Stdout = &checkoutStdout
	checkoutCmd.Stderr = &checkoutStderr
	if err := checkoutCmd.Run(); err != nil {
		fmt.Printf("DEBUG: Failed to create/checkout branch. Error: %v\n", err)
		fmt.Printf("DEBUG: git checkout stdout: %s\n", checkoutStdout.String())
		fmt.Printf("DEBUG: git checkout stderr: %s\n", checkoutStderr.String())
		return fmt.Errorf("failed to create/checkout branch: %w", err)
	}
	fmt.Println("DEBUG: Successfully created and checked out branch")

	// Push changes using credentials from environment variables
	username := os.Getenv("GITLAB_USERNAME")
	token := os.Getenv("GITLAB_TOKEN")
	if username == "" || token == "" {
		return fmt.Errorf("GITLAB_USERNAME and GITLAB_TOKEN environment variables must be set")
	}

	repoURL := fmt.Sprintf("https://%s:%s@gitlab.cee.redhat.com/sashture/konflux-release-data.git", username, token)
	fmt.Printf("DEBUG: Pushing to repository with URL: %s\n", strings.Replace(repoURL, token, "[REDACTED]", 1))

	pushCmd := exec.Command("git", "push", "-u", repoURL, branchName)
	pushCmd.Dir = config.RepoPath
	var pushStdout, pushStderr bytes.Buffer
	pushCmd.Stdout = &pushStdout
	pushCmd.Stderr = &pushStderr
	if err := pushCmd.Run(); err != nil {
		fmt.Printf("DEBUG: Failed to push changes. Error: %v\n", err)
		fmt.Printf("DEBUG: git push stdout: %s\n", pushStdout.String())
		fmt.Printf("DEBUG: git push stderr: %s\n", pushStderr.String())
		return fmt.Errorf("failed to push changes: %w", err)
	}
	fmt.Println("DEBUG: Successfully pushed changes")

	fmt.Printf("DEBUG: Changes have been pushed to branch '%s'. Please create merge request manually via GitLab UI.\n", branchName)
	return nil
}

func getReleaseType(minorVersion, patchVersion string) (string, string) {
	if patchVersion != "" {
		return "RHBA", fmt.Sprintf("%s.%s", minorVersion, patchVersion)
	}
	return "RHEA", fmt.Sprintf("%s.0", minorVersion)
}

// func AddReleasePlanTool(_ context.Context, s *mcp.Server) error {
// 	tool := &mcp.Tool{
// 		Name:        "create-release-plans",
// 		Description: "Creates ReleasePlanAdmission and ReleasePlan files for Tekton components",
// 	}

// 	handler := func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
// 		// Extract parameters
// 		minorVersion, ok := params.Arguments["minor_version"].(string)
// 		if !ok || minorVersion == "" {
// 			return nil, fmt.Errorf("minor_version parameter is required")
// 		}

// 		// Patch version is optional
// 		patchVersion, _ := params.Arguments["patch_version"].(string)

// 		// Get OCP versions from input or use defaults
// 		var ocpVersions []string
// 		if versions, ok := params.Arguments["ocp_versions"].([]interface{}); ok && len(versions) > 0 {
// 			for _, v := range versions {
// 				if strVal, ok := v.(string); ok {
// 					ocpVersions = append(ocpVersions, strVal)
// 				}
// 			}
// 		}
// 		if len(ocpVersions) == 0 {
// 			ocpVersions = []string{"4-15", "4-16", "4-17", "4-18", "4-19"}
// 		}

// 		// Define component configurations
// 		components := map[string][]ComponentConfig{
// 			"cli": {
// 				{Name: "tkn", Repository: "pipelines-cli-tkn-rhel9"},
// 			},
// 			"core": {
// 				{Name: "controller", Repository: "pipelines-core-controller-rhel9"},
// 				{Name: "webhook", Repository: "pipelines-core-webhook-rhel9"},
// 			},
// 			"operator": {
// 				{Name: "operator", Repository: "pipelines-rhel9-operator"},
// 				{Name: "proxy", Repository: "pipelines-operator-proxy-rhel9"},
// 				{Name: "webhook", Repository: "pipelines-operator-webhook-rhel9"},
// 			},
// 			"fbc": {}, // FBC has special handling
// 		}

// 		config := RPAConfig{
// 			MinorVersion: minorVersion,
// 			PatchVersion: patchVersion,
// 			RepoPath:     filepath.Join(os.TempDir(), "konflux-release-data"),
// 			Components:   components,
// 			Environments: []string{"stage", "prod"},
// 			OCPVersions:  ocpVersions,
// 		}

// 		if err := createReleasePlans(config); err != nil {
// 			return &mcp.CallToolResultFor[any]{
// 				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create release plans: %v", err)}},
// 			}, nil
// 		}

// 		return &mcp.CallToolResultFor[any]{
// 			Content: []mcp.Content{&mcp.TextContent{Text: "Successfully created ReleasePlan and ReleasePlanAdmission files"}},
// 		}, nil
// 	}

// 	s.AddTool(tool, handler)
// 	return nil
// }
