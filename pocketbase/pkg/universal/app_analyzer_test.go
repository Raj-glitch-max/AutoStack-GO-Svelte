package universal

import (
	"testing"
)

func TestLanguageDetection(t *testing.T) {
	cases := []struct {
		name     string
		files    []string
		expected string
	}{
		{"nodejs express", []string{"package.json", "index.js"}, "nodejs"},
		{"python fastapi", []string{"requirements.txt", "main.py"}, "python"},
		{"python poetry", []string{"pyproject.toml", "app.py"}, "python"},
		{"go gin", []string{"go.mod", "main.go"}, "go"},
		{"java maven", []string{"pom.xml", "src/main/java/App.java"}, "java"},
		{"java gradle", []string{"build.gradle", "src/main/java/App.java"}, "java"},
		{"php laravel", []string{"composer.json", "artisan"}, "php"},
		{"ruby rails", []string{"Gemfile", "config/routes.rb"}, "ruby"},
		{"static site", []string{"index.html", "styles.css"}, "static"},
		{"rust", []string{"Cargo.toml", "src/main.rs"}, "rust"},
		{"elixir", []string{"mix.exs", "lib/app.ex"}, "elixir"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectLanguage(tc.files)
			if got != tc.expected {
				t.Errorf("detectLanguage(%v) = %q, want %q", tc.files, got, tc.expected)
			}
		})
	}
}

func TestStaticSiteProfile(t *testing.T) {
	raw := RawAppDescription{
		InputType:   "source",
		Files:       []string{"index.html", "styles.css", "app.js"},
		ProjectName: "my-site",
	}
	profile := AnalyzeApp(raw)

	if !profile.IsStaticSite {
		t.Error("expected IsStaticSite=true")
	}
	if profile.AppType != AppTypeStaticSite {
		t.Errorf("expected AppType=static_site, got %s", profile.AppType)
	}
	if profile.ResourceProfile != ResourceMinimal {
		t.Errorf("expected ResourceMinimal, got %s", profile.ResourceProfile)
	}
}

func TestDefaultPort(t *testing.T) {
	raw := RawAppDescription{
		InputType:   "source",
		Files:       []string{"go.mod", "main.go"},
		ProjectName: "my-api",
	}
	profile := AnalyzeApp(raw)
	if profile.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", profile.Port)
	}
}

func TestDependencyDetection(t *testing.T) {
	depText := "psycopg2==2.9.9\nredis==5.0.0\nfastapi==0.110.0"
	services := detectDependencies(depText)

	found := map[string]bool{}
	for _, s := range services {
		found[s.ServiceType] = true
	}

	if !found["postgresql"] {
		t.Error("expected postgresql service detected from psycopg2")
	}
	if !found["redis"] {
		t.Error("expected redis service detected")
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"My App", "my-app"},
		{"my_app_123", "my-app-123"},
		{"---test---", "test"},
		{"", "autostack-app"},
		{"a very long name that exceeds thirty two characters total", "a-very-long-name-that-exceeds-th"},
	}
	for _, tc := range cases {
		got := sanitizeName(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
