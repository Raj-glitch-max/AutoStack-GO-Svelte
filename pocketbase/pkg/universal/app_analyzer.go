package universal

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AnalyzeApp converts a RawAppDescription into a structured AppProfile.
// Uses deterministic rules first; falls back to AI for ambiguous cases.
func AnalyzeApp(raw RawAppDescription) AppProfile {
	profile := AppProfile{
		ProjectName:         raw.ProjectName,
		Port:                8080, // safe default
		DetectionConfidence: 1.0,
	}

	// ── Language detection ────────────────────────────────────────────
	profile.Language = detectLanguage(raw.Files)
	if profile.Language == "" {
		// Try base image if we have a Docker image
		if raw.DockerImage != "" {
			profile.Language = languageFromImage(raw.DockerImage)
		}
	}
	if profile.Language == "" {
		profile.Language = "nodejs" // safest default
		profile.DetectionConfidence = 0.3
		profile.AmbiguousFields = append(profile.AmbiguousFields, "language")
	}

	// ── Static site shortcut ──────────────────────────────────────────
	if profile.Language == "static" {
		profile.IsStaticSite = true
		profile.AppType = AppTypeStaticSite
		profile.ExpectsWebTraffic = true
		profile.ResourceProfile = ResourceMinimal
		return profile
	}

	// ── Framework detection ───────────────────────────────────────────
	profile.Framework, profile.AppType = detectFramework(profile.Language, raw.Files, raw.SourceDir)
	if profile.AppType == "" {
		profile.AppType = AppTypeAPI // safe default
		profile.AmbiguousFields = append(profile.AmbiguousFields, "app_type")
		profile.DetectionConfidence -= 0.1
	}

	// ── Port detection ────────────────────────────────────────────────
	if raw.DockerfilePath != "" {
		if port := detectPortFromDockerfile(raw.DockerfilePath); port > 0 {
			profile.Port = port
		}
	} else if raw.SourceDir != "" {
		if port := detectPortFromSource(raw.SourceDir, profile.Language); port > 0 {
			profile.Port = port
		}
	}

	// ── Dockerfile presence ───────────────────────────────────────────
	if raw.DockerfilePath != "" {
		profile.HasDockerfile = true
		profile.DockerfilePath = raw.DockerfilePath
	} else if fileExists(raw.Files, "Dockerfile") {
		profile.HasDockerfile = true
		profile.DockerfilePath = filepath.Join(raw.SourceDir, "Dockerfile")
	}

	// ── Dependency detection ──────────────────────────────────────────
	deps := collectDependencyText(raw.SourceDir, profile.Language)
	profile.RequiredServices = detectDependencies(deps)
	profile.EnvVarKeys = detectEnvVarKeys(raw.SourceDir)

	// ── Storage needs ─────────────────────────────────────────────────
	profile.NeedsObjectStorage = containsAny(deps, []string{"s3", "boto3", "minio", "@aws-sdk/client-s3"})
	profile.NeedsFileStorage = detectFileStorageNeed(raw.SourceDir, profile.Language)

	// ── Traffic patterns ──────────────────────────────────────────────
	profile.ExpectsWebTraffic = profile.AppType != AppTypeWorker && profile.AppType != AppTypeScheduled
	profile.ExpectsWebSocket = profile.AppType == AppTypeWebSocket
	profile.IsBackgroundWorker = profile.AppType == AppTypeWorker
	profile.IsScheduledJob = profile.AppType == AppTypeScheduled

	// ── Resource profile ──────────────────────────────────────────────
	profile.ResourceProfile = inferResourceProfile(profile)

	// ── Confidence adjustment ─────────────────────────────────────────
	if len(profile.AmbiguousFields) > 2 {
		profile.DetectionConfidence = 0.5
	}

	return profile
}

// detectLanguage returns the language based on indicator files
func detectLanguage(files []string) string {
	for _, detector := range LanguageDetectors {
		for _, indicatorFile := range detector.Files {
			if fileExists(files, indicatorFile) {
				return detector.Language
			}
		}
	}
	return ""
}

// languageFromImage infers language from a Docker base image name
func languageFromImage(image string) string {
	lower := strings.ToLower(image)
	for prefix, lang := range BaseImageLanguageMap {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower, "/"+prefix+":") {
			return lang
		}
	}
	return ""
}

// detectFramework returns the framework and app type for a given language
func detectFramework(language string, files []string, sourceDir string) (string, AppType) {
	detectors, ok := FrameworkDetectors[language]
	if !ok {
		return "", ""
	}

	// Read package.json for Node.js
	var packageDeps string
	if language == "nodejs" {
		packageDeps = readFileContent(filepath.Join(sourceDir, "package.json"))
	}

	// Read go.mod for Go
	var goMod string
	if language == "go" {
		goMod = readFileContent(filepath.Join(sourceDir, "go.mod"))
	}

	// Read requirements.txt for Python
	var requirements string
	if language == "python" {
		requirements = readFileContent(filepath.Join(sourceDir, "requirements.txt"))
		requirements += readFileContent(filepath.Join(sourceDir, "pyproject.toml"))
	}

	// Read Gemfile for Ruby
	var gemfile string
	if language == "ruby" {
		gemfile = readFileContent(filepath.Join(sourceDir, "Gemfile"))
	}

	// Read composer.json for PHP
	var composerJSON string
	if language == "php" {
		composerJSON = readFileContent(filepath.Join(sourceDir, "composer.json"))
	}

	for _, d := range detectors {
		switch {
		case d.PackageKey != "" && language == "nodejs":
			if strings.Contains(packageDeps, `"`+d.PackageKey+`"`) {
				return d.Framework, d.AppType
			}
		case d.PackageKey != "" && language == "ruby":
			if strings.Contains(gemfile, `'`+d.PackageKey+`'`) || strings.Contains(gemfile, `"`+d.PackageKey+`"`) {
				return d.Framework, d.AppType
			}
		case d.PackageKey != "" && language == "php":
			if strings.Contains(composerJSON, `"`+d.PackageKey+`"`) {
				return d.Framework, d.AppType
			}
		case d.ImportPattern != "" && language == "go":
			if strings.Contains(goMod, d.ImportPattern) {
				return d.Framework, d.AppType
			}
		case d.ImportPattern != "" && language == "python":
			if strings.Contains(requirements, d.ImportPattern) {
				return d.Framework, d.AppType
			}
		case d.FilePattern != "":
			if fileExists(files, d.FilePattern) {
				return d.Framework, d.AppType
			}
		}
	}

	return "", ""
}

// detectPortFromDockerfile extracts the EXPOSE port from a Dockerfile
func detectPortFromDockerfile(dockerfilePath string) int {
	content := readFileContent(dockerfilePath)
	re := regexp.MustCompile(`EXPOSE\s+(\d{4,5})`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		port := 0
		for _, c := range matches[1] {
			port = port*10 + int(c-'0')
		}
		return port
	}
	return 0
}

// detectPortFromSource scans source files for port patterns
func detectPortFromSource(sourceDir string, language string) int {
	for _, pattern := range PortPatterns {
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			continue
		}

		// Walk source files looking for the pattern
		filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// Only scan relevant files
			ext := filepath.Ext(path)
			if !isSourceFile(ext, language) {
				return nil
			}
			content := readFileContent(path)
			matches := re.FindStringSubmatch(content)
			if len(matches) > 1 {
				// Found a port — return it (we'll use the first one found)
				return filepath.SkipAll
			}
			return nil
		})
	}
	return 0
}

// collectDependencyText reads all dependency files and returns their combined content
func collectDependencyText(sourceDir string, language string) string {
	var parts []string
	depFiles := map[string][]string{
		"nodejs":  {"package.json"},
		"python":  {"requirements.txt", "pyproject.toml", "Pipfile"},
		"go":      {"go.mod", "go.sum"},
		"ruby":    {"Gemfile", "Gemfile.lock"},
		"php":     {"composer.json"},
		"java":    {"pom.xml", "build.gradle"},
		"rust":    {"Cargo.toml"},
		"elixir":  {"mix.exs"},
	}

	files, ok := depFiles[language]
	if !ok {
		return ""
	}

	for _, f := range files {
		content := readFileContent(filepath.Join(sourceDir, f))
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n")
}

// detectDependencies returns required managed services based on dependency text
func detectDependencies(depText string) []RequiredService {
	var services []RequiredService
	seen := map[string]bool{}

	lower := strings.ToLower(depText)
	for _, pattern := range DependencyPatterns {
		if pattern.Service == "" || pattern.Service == "sqlite" {
			continue // sqlite runs in-container, no managed service needed
		}
		for _, p := range pattern.Patterns {
			if strings.Contains(lower, strings.ToLower(p)) {
				if !seen[pattern.Service] {
					seen[pattern.Service] = true
					services = append(services, RequiredService{
						ServiceType: pattern.Service,
						IsCritical:  true,
						EnvVarName:  defaultEnvVarForService(pattern.Service),
					})
				}
				break
			}
		}
	}
	return services
}

// detectEnvVarKeys reads .env.example or .env files and returns the key names
func detectEnvVarKeys(sourceDir string) []string {
	var keys []string
	for _, f := range []string{".env.example", ".env.sample", ".env"} {
		content := readFileContent(filepath.Join(sourceDir, f))
		if content == "" {
			continue
		}
		scanner := bufio.NewScanner(strings.NewReader(content))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				keys = append(keys, line[:idx])
			}
		}
	}
	return keys
}

// detectFileStorageNeed checks if the app reads/writes files that must persist
func detectFileStorageNeed(sourceDir string, language string) bool {
	// Look for common file upload patterns
	patterns := []string{
		"multer", "formidable", "busboy", // Node.js file upload
		"FileField", "ImageField",         // Django
		"werkzeug.utils.secure_filename",  // Flask
		"multipart/form-data",
		"os.WriteFile", "ioutil.WriteFile", // Go file writes
		"open(", "write(",                  // Python file writes
	}

	depText := collectDependencyText(sourceDir, language)
	return containsAny(depText, patterns)
}

// inferResourceProfile determines the resource profile from the app profile
func inferResourceProfile(profile AppProfile) ResourceProfile {
	switch {
	case profile.AppType == AppTypeMLInference:
		return ResourceCompute
	case profile.Framework == "tensorflow" || profile.Framework == "pytorch":
		return ResourceGPU
	case profile.AppType == AppTypeWorker:
		return ResourceStandard
	case profile.IsStaticSite:
		return ResourceMinimal
	default:
		return ResourceStandard
	}
}

// defaultEnvVarForService returns the conventional env var name for a service
func defaultEnvVarForService(serviceType string) string {
	m := map[string]string{
		"postgresql": "DATABASE_URL",
		"mysql":      "DATABASE_URL",
		"mongodb":    "MONGODB_URL",
		"redis":      "REDIS_URL",
		"rabbitmq":   "RABBITMQ_URL",
		"kafka":      "KAFKA_BROKERS",
		"opensearch": "OPENSEARCH_URL",
		"s3":         "S3_BUCKET_NAME",
	}
	if v, ok := m[serviceType]; ok {
		return v
	}
	return strings.ToUpper(serviceType) + "_URL"
}

// isSourceFile returns true if the file extension is a source file for the language
func isSourceFile(ext string, language string) bool {
	m := map[string][]string{
		"nodejs":  {".js", ".ts", ".mjs", ".cjs"},
		"python":  {".py"},
		"go":      {".go"},
		"java":    {".java"},
		"ruby":    {".rb"},
		"php":     {".php"},
		"rust":    {".rs"},
		"elixir":  {".ex", ".exs"},
	}
	exts, ok := m[language]
	if !ok {
		return false
	}
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// readFileContent reads a file and returns its content, or empty string on error
func readFileContent(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
