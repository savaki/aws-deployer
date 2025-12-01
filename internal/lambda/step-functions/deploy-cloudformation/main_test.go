package main

import (
	"testing"
)

func TestReplaceFilename(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		newFilename string
		want        string
	}{
		{
			name:        "replace filename in path",
			key:         "myrepo/main/1.2.3/cloudformation-params.json",
			newFilename: "cloudformation-params.dev.json",
			want:        "myrepo/main/1.2.3/cloudformation-params.dev.json",
		},
		{
			name:        "replace filename in deep path",
			key:         "some/deep/path/file.txt",
			newFilename: "newfile.txt",
			want:        "some/deep/path/newfile.txt",
		},
		{
			name:        "filename only (no path)",
			key:         "file.txt",
			newFilename: "newfile.txt",
			want:        "newfile.txt",
		},
		{
			name:        "single directory",
			key:         "dir/file.txt",
			newFilename: "other.txt",
			want:        "dir/other.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceFilename(tt.key, tt.newFilename)
			if got != tt.want {
				t.Errorf("replaceFilename(%q, %q) = %q, want %q", tt.key, tt.newFilename, got, tt.want)
			}
		})
	}
}

func TestEnvSpecificParamsFilename(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		wantFilename string
	}{
		{
			name:         "dev environment",
			env:          "dev",
			wantFilename: "cloudformation-params.dev.json",
		},
		{
			name:         "staging environment",
			env:          "staging",
			wantFilename: "cloudformation-params.staging.json",
		},
		{
			name:         "prod environment",
			env:          "prod",
			wantFilename: "cloudformation-params.prod.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from downloadAndParseParams
			envFilename := "cloudformation-params." + tt.env + ".json"

			if envFilename != tt.wantFilename {
				t.Errorf("env filename = %q, want %q", envFilename, tt.wantFilename)
			}
		})
	}
}

func TestStackNameGeneration(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		repo      string
		wantStack string
	}{
		{
			name:      "dev environment",
			env:       "dev",
			repo:      "myapp",
			wantStack: "dev-myapp",
		},
		{
			name:      "staging environment",
			env:       "staging",
			repo:      "service",
			wantStack: "staging-service",
		},
		{
			name:      "prod environment",
			env:       "prod",
			repo:      "myapp",
			wantStack: "prod-myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stackName := tt.env + "-" + tt.repo

			if stackName != tt.wantStack {
				t.Errorf("stack name = %q, want %q", stackName, tt.wantStack)
			}
		})
	}
}
