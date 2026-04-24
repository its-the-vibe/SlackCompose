package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Run("existing env var", func(t *testing.T) {
		t.Setenv("TEST_VAR_ABC", "hello")
		if got := getEnv("TEST_VAR_ABC", "default"); got != "hello" {
			t.Errorf("getEnv() = %q, want %q", got, "hello")
		}
	})

	t.Run("missing env var returns default", func(t *testing.T) {
		os.Unsetenv("MISSING_VAR_XYZ")
		if got := getEnv("MISSING_VAR_XYZ", "default"); got != "default" {
			t.Errorf("getEnv() = %q, want %q", got, "default")
		}
	})
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue int
		want         int
	}{
		{"valid integer", "42", 0, 42},
		{"missing env var uses default", "", 10, 10},
		{"invalid value uses default", "abc", 5, 5},
		{"zero value", "0", 99, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_INT_VAR_XYZ"
			if tt.envValue != "" {
				t.Setenv(key, tt.envValue)
			} else {
				os.Unsetenv(key)
			}
			if got := getEnvInt(key, tt.defaultValue); got != tt.want {
				t.Errorf("getEnvInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoadProjectConfig_ValidFile(t *testing.T) {
	projects := []ProjectConfig{
		{Name: "project-a", WorkingDir: "/path/to/a"},
		{Name: "project-b", WorkingDir: "/path/to/b"},
	}
	data, err := json.Marshal(projects)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	f, err := os.CreateTemp("", "projects*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Write(data)
	f.Close()

	config := &Config{ProjectConfigPath: f.Name()}
	if err := config.loadProjectConfig(); err != nil {
		t.Fatalf("loadProjectConfig() error = %v", err)
	}

	if len(config.Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(config.Projects))
	}
	if p, ok := config.Projects["project-a"]; !ok || p.WorkingDir != "/path/to/a" {
		t.Error("project-a not loaded correctly")
	}
	if p, ok := config.Projects["project-b"]; !ok || p.WorkingDir != "/path/to/b" {
		t.Error("project-b not loaded correctly")
	}
}

func TestLoadProjectConfig_MissingFile(t *testing.T) {
	config := &Config{ProjectConfigPath: "/nonexistent/path/projects.json"}
	if err := config.loadProjectConfig(); err != nil {
		t.Fatalf("loadProjectConfig() with missing file should not error, got %v", err)
	}
	if config.Projects == nil {
		t.Error("expected empty map, got nil")
	}
	if len(config.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(config.Projects))
	}
}

func TestLoadProjectConfig_InvalidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "projects*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("not valid json")
	f.Close()

	config := &Config{ProjectConfigPath: f.Name()}
	if err := config.loadProjectConfig(); err == nil {
		t.Error("loadProjectConfig() with invalid JSON should return error")
	}
}

func TestLoadProjectConfig_EmptyArray(t *testing.T) {
	f, err := os.CreateTemp("", "projects*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("[]")
	f.Close()

	config := &Config{ProjectConfigPath: f.Name()}
	if err := config.loadProjectConfig(); err != nil {
		t.Fatalf("loadProjectConfig() error = %v", err)
	}
	if len(config.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(config.Projects))
	}
}
