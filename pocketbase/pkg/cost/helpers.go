package cost

import "math"

// Helper functions for blueprint calculators

// roundToTwoDecimals rounds a float64 value to 2 decimal places
func roundToTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

// getFloat64OrDefault gets a float64 value from a map or returns default
func getFloat64OrDefault(m map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return defaultValue
}

// getIntOrDefault gets an int value from a map or returns default
func getIntOrDefault(m map[string]interface{}, key string, defaultValue int) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// getStringOrDefault gets a string value from a map or returns default
func getStringOrDefault(m map[string]interface{}, key string, defaultValue string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// getBoolOrDefault gets a bool value from a map or returns default
func getBoolOrDefault(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case string:
			return v == "true" || v == "yes" || v == "1"
		}
	}
	return defaultValue
}

// containsAll checks if a string contains all of the given substrings
func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !containsStr(s, sub) {
			return false
		}
	}
	return true
}

// containsStr is a simple case-insensitive contains check
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
