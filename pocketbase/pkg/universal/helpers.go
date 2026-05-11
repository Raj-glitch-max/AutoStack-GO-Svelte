package universal

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// sanitizeName converts a project name to a safe AWS resource name prefix.
// AWS resource names must be lowercase alphanumeric + hyphens, max 32 chars.
func sanitizeName(name string) string {
	// Lowercase
	name = strings.ToLower(name)
	// Replace non-alphanumeric with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	name = re.ReplaceAllString(name, "-")
	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")
	// Truncate to 32 chars
	if len(name) > 32 {
		name = name[:32]
	}
	if name == "" {
		name = "autostack-app"
	}
	return name
}

// randomSuffix returns a 6-character random hex string for unique resource names
func randomSuffix() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// coalesce returns the first non-empty string
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// resourceToCPU maps a ResourceProfile to ECS Fargate CPU units
func resourceToCPU(r ResourceProfile) int {
	switch r {
	case ResourceMinimal:
		return 256
	case ResourceStandard:
		return 512
	case ResourceCompute:
		return 2048
	case ResourceMemory:
		return 1024
	case ResourceGPU:
		return 4096
	default:
		return 512
	}
}

// resourceToMemory maps a ResourceProfile to ECS Fargate memory in MB
func resourceToMemory(r ResourceProfile) int {
	switch r {
	case ResourceMinimal:
		return 512
	case ResourceStandard:
		return 1024
	case ResourceCompute:
		return 4096
	case ResourceMemory:
		return 8192
	case ResourceGPU:
		return 16384
	default:
		return 1024
	}
}

// scalingMaxFromPrefs returns the max task count from user preferences
func scalingMaxFromPrefs(prefs UserPreferences) int {
	if prefs.MaxTasks > 0 {
		return prefs.MaxTasks
	}
	if prefs.HighAvailability {
		return 10
	}
	return 5
}

// dbInstanceFromPrefs returns the RDS instance class based on user preferences
func dbInstanceFromPrefs(prefs UserPreferences) string {
	if prefs.HighAvailability {
		return "db.t3.small"
	}
	return "db.t3.micro"
}

// containsAny checks if any of the patterns appear in the text (case-insensitive)
func containsAny(text string, patterns []string) bool {
	lower := strings.ToLower(text)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// fileExists checks if a filename (or glob pattern) exists in a file list
func fileExists(files []string, name string) bool {
	for _, f := range files {
		// Exact match
		if f == name || strings.HasSuffix(f, "/"+name) {
			return true
		}
		// Basename match
		parts := strings.Split(f, "/")
		if parts[len(parts)-1] == name {
			return true
		}
	}
	return false
}

// anyFileMatches checks if any file in the list matches a suffix pattern
func anyFileMatches(files []string, suffix string) bool {
	for _, f := range files {
		if strings.HasSuffix(f, suffix) {
			return true
		}
	}
	return false
}
