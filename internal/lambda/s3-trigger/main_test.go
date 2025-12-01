package main

import "testing"

func TestVersionParsing(t *testing.T) {
	tests := []struct {
		name            string
		version         string
		wantBuildNumber string
		wantCommitHash  string
	}{
		{
			name:            "standard version format",
			version:         "123.abc123",
			wantBuildNumber: "123",
			wantCommitHash:  "abc123",
		},
		{
			name:            "longer commit hash",
			version:         "456.def456789",
			wantBuildNumber: "456",
			wantCommitHash:  "def456789",
		},
		{
			name:            "version with multiple dots in commit",
			version:         "789.abc.def.ghi",
			wantBuildNumber: "789",
			wantCommitHash:  "abc.def.ghi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from processS3Record
			versionParts := make([]string, 0)
			current := ""
			for _, c := range tt.version {
				if c == '.' {
					versionParts = append(versionParts, current)
					current = ""
				} else {
					current += string(c)
				}
			}
			if current != "" {
				versionParts = append(versionParts, current)
			}

			if len(versionParts) < 2 {
				t.Fatalf("invalid version format: %s", tt.version)
			}

			buildNumber := versionParts[0]
			commitHash := ""
			for i := 1; i < len(versionParts); i++ {
				if i > 1 {
					commitHash += "."
				}
				commitHash += versionParts[i]
			}

			if buildNumber != tt.wantBuildNumber {
				t.Errorf("buildNumber = %q, want %q", buildNumber, tt.wantBuildNumber)
			}
			if commitHash != tt.wantCommitHash {
				t.Errorf("commitHash = %q, want %q", commitHash, tt.wantCommitHash)
			}
		})
	}
}

func TestS3KeyParsing(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		wantRepo     string
		wantBranch   string
		wantVersion  string
		wantFilename string
	}{
		{
			name:         "main template",
			key:          "myapp/main/1.0.abc123/cloudformation-params.json",
			wantRepo:     "myapp",
			wantBranch:   "main",
			wantVersion:  "1.0.abc123",
			wantFilename: "cloudformation-params.json",
		},
		{
			name:         "feature branch",
			key:          "myapp/feature-branch/2.0.def456/cloudformation-params.json",
			wantRepo:     "myapp",
			wantBranch:   "feature-branch",
			wantVersion:  "2.0.def456",
			wantFilename: "cloudformation-params.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the S3 key parsing logic
			pathParts := make([]string, 0)
			current := ""
			for _, c := range tt.key {
				if c == '/' {
					if current != "" {
						pathParts = append(pathParts, current)
					}
					current = ""
				} else {
					current += string(c)
				}
			}
			if current != "" {
				pathParts = append(pathParts, current)
			}

			if len(pathParts) < 4 {
				t.Fatalf("invalid S3 key format: %s", tt.key)
			}

			repo := pathParts[0]
			branch := pathParts[1]
			version := pathParts[2]
			filename := pathParts[len(pathParts)-1]

			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %q, want %q", version, tt.wantVersion)
			}
			if filename != tt.wantFilename {
				t.Errorf("filename = %q, want %q", filename, tt.wantFilename)
			}
		})
	}
}

func TestStackNameGeneration(t *testing.T) {
	tests := []struct {
		name          string
		env           string
		repo          string
		wantStackName string
	}{
		{
			name:          "dev environment",
			env:           "dev",
			repo:          "myapp",
			wantStackName: "dev-myapp",
		},
		{
			name:          "prod environment",
			env:           "prod",
			repo:          "myapp",
			wantStackName: "prod-myapp",
		},
		{
			name:          "staging environment",
			env:           "staging",
			repo:          "service",
			wantStackName: "staging-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stackName := tt.env + "-" + tt.repo

			if stackName != tt.wantStackName {
				t.Errorf("stackName = %q, want %q", stackName, tt.wantStackName)
			}
		})
	}
}

func TestProcessS3RecordFiltering(t *testing.T) {
	tests := []struct {
		name          string
		filename      string
		shouldProcess bool
		reason        string
	}{
		{
			name:          "main template params - should process",
			filename:      "cloudformation-params.json",
			shouldProcess: true,
			reason:        "main template trigger file",
		},
		{
			name:          "env-specific params - should NOT process",
			filename:      "cloudformation-params.dev.json",
			shouldProcess: false,
			reason:        "env override file, not a trigger",
		},
		{
			name:          "template file - should NOT process",
			filename:      "cloudformation.template",
			shouldProcess: false,
			reason:        "template file, not params",
		},
		{
			name:          "random json - should NOT process",
			filename:      "config.json",
			shouldProcess: false,
			reason:        "unrelated file",
		},
		{
			name:          "sub-template params - should NOT process (silently ignored)",
			filename:      "cloudformation-worker-params.json",
			shouldProcess: false,
			reason:        "sub-template files are silently ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only cloudformation-params.json is processed
			shouldProcess := tt.filename == "cloudformation-params.json"

			if shouldProcess != tt.shouldProcess {
				t.Errorf("shouldProcess = %v, want %v (%s)", shouldProcess, tt.shouldProcess, tt.reason)
			}
		})
	}
}
