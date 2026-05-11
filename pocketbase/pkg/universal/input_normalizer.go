package universal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NormalizeInput converts any input type into a RawAppDescription.
// After this stage, all subsequent logic is input-type-agnostic.
func NormalizeInput(req UniversalDeployRequest, workDir string) (*RawAppDescription, error) {
	raw := &RawAppDescription{
		InputType:   req.InputType,
		ProjectName: deriveProjectName(req),
	}

	switch req.InputType {
	case "git":
		return normalizeGit(req.GitURL, workDir, raw)
	case "image":
		return normalizeImage(req.DockerImage, raw)
	case "compose":
		return normalizeCompose(req.ComposeContent, raw)
	case "natural_language":
		raw.NaturalLanguage = req.NaturalLanguage
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported input type: %s", req.InputType)
	}
}

// normalizeGit clones a git repo and scans it like source code
func normalizeGit(gitURL string, workDir string, raw *RawAppDescription) (*RawAppDescription, error) {
	if gitURL == "" {
		return nil, fmt.Errorf("git_url is required for input_type=git")
	}

	sourceDir := filepath.Join(workDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create source dir: %w", err)
	}

	// Clone with depth=1 for speed
	cmd := exec.Command("git", "clone", "--depth=1", gitURL, sourceDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}

	raw.SourceDir = sourceDir
	raw.Files = scanDirectory(sourceDir)

	// Check for Dockerfile
	dockerfilePath := filepath.Join(sourceDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		raw.DockerfilePath = dockerfilePath
	}

	// Derive project name from repo URL if not set
	if raw.ProjectName == "my-app" {
		parts := strings.Split(strings.TrimSuffix(gitURL, ".git"), "/")
		if len(parts) > 0 {
			raw.ProjectName = parts[len(parts)-1]
		}
	}

	return raw, nil
}

// normalizeImage extracts metadata from a Docker image name
func normalizeImage(image string, raw *RawAppDescription) (*RawAppDescription, error) {
	if image == "" {
		return nil, fmt.Errorf("docker_image is required for input_type=image")
	}

	raw.DockerImage = image

	// Derive project name from image name
	if raw.ProjectName == "my-app" {
		parts := strings.Split(image, ":")
		nameParts := strings.Split(parts[0], "/")
		raw.ProjectName = nameParts[len(nameParts)-1]
	}

	return raw, nil
}

// normalizeCompose parses a docker-compose.yml and identifies services
func normalizeCompose(content string, raw *RawAppDescription) (*RawAppDescription, error) {
	if content == "" {
		return nil, fmt.Errorf("compose_content is required for input_type=compose")
	}

	var compose struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Build       interface{}       `yaml:"build"`
			Ports       []string          `yaml:"ports"`
			Environment interface{}       `yaml:"environment"`
			DependsOn   interface{}       `yaml:"depends_on"`
			Volumes     []string          `yaml:"volumes"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal([]byte(content), &compose); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose.yml: %w", err)
	}

	for name, svc := range compose.Services {
		cs := ComposeService{
			Name:    name,
			Image:   svc.Image,
			Ports:   svc.Ports,
			Volumes: svc.Volumes,
		}

		// Parse environment (can be map or list)
		cs.Environment = parseComposeEnv(svc.Environment)

		// Parse depends_on (can be list or map)
		cs.DependsOn = parseComposeDependsOn(svc.DependsOn)

		// Parse build path
		if svc.Build != nil {
			switch v := svc.Build.(type) {
			case string:
				cs.BuildPath = v
			case map[string]interface{}:
				if ctx, ok := v["context"].(string); ok {
					cs.BuildPath = ctx
				}
			}
		}

		// Check if this is a well-known managed service
		if managedType, ok := WellKnownServiceNames[strings.ToLower(name)]; ok && managedType != "" {
			cs.IsDatabase = true
			cs.IsManagedByAWS = true
			cs.ManagedServiceType = managedType
		}

		raw.ComposeServices = append(raw.ComposeServices, cs)
	}

	return raw, nil
}

// scanDirectory returns a list of all file paths in a directory (relative)
func scanDirectory(dir string) []string {
	var files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip common non-source directories
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" ||
				name == "__pycache__" || name == ".venv" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, rel)
		return nil
	})
	return files
}

// deriveProjectName extracts a project name from the request
func deriveProjectName(req UniversalDeployRequest) string {
	if req.GitURL != "" {
		parts := strings.Split(strings.TrimSuffix(req.GitURL, ".git"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return parts[len(parts)-1]
		}
	}
	if req.DockerImage != "" {
		parts := strings.Split(req.DockerImage, ":")
		nameParts := strings.Split(parts[0], "/")
		return nameParts[len(nameParts)-1]
	}
	return "my-app"
}

// parseComposeEnv handles both map and list formats for docker-compose environment
func parseComposeEnv(env interface{}) map[string]string {
	result := make(map[string]string)
	if env == nil {
		return result
	}
	switch v := env.(type) {
	case map[string]interface{}:
		for key, val := range v {
			result[key] = fmt.Sprintf("%v", val)
		}
	case []interface{}:
		for _, item := range v {
			s := fmt.Sprintf("%v", item)
			if idx := strings.Index(s, "="); idx > 0 {
				result[s[:idx]] = s[idx+1:]
			} else {
				result[s] = ""
			}
		}
	}
	return result
}

// parseComposeDependsOn handles both list and map formats for depends_on
func parseComposeDependsOn(deps interface{}) []string {
	var result []string
	if deps == nil {
		return result
	}
	switch v := deps.(type) {
	case []interface{}:
		for _, item := range v {
			result = append(result, fmt.Sprintf("%v", item))
		}
	case map[string]interface{}:
		for key := range v {
			result = append(result, key)
		}
	}
	return result
}
