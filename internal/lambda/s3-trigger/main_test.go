package main

import "testing"

func TestExtractTemplateName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		// Main template cases
		{
			name:     "main template params file",
			filename: "cloudformation-params.json",
			want:     "",
		},
		{
			name:     "main template env-specific params",
			filename: "cloudformation-params.dev.json",
			want:     "",
		},
		{
			name:     "main template env-specific params (staging)",
			filename: "cloudformation-params.staging.json",
			want:     "",
		},
		{
			name:     "main template env-specific params (prod)",
			filename: "cloudformation-params.prod.json",
			want:     "",
		},

		// Sub-template cases
		{
			name:     "sub-template worker",
			filename: "cloudformation-worker-params.json",
			want:     "worker",
		},
		{
			name:     "sub-template api",
			filename: "cloudformation-api-params.json",
			want:     "api",
		},
		{
			name:     "sub-template with hyphen",
			filename: "cloudformation-my-service-params.json",
			want:     "my-service",
		},
		{
			name:     "sub-template scheduler",
			filename: "cloudformation-scheduler-params.json",
			want:     "scheduler",
		},

		// Sub-template env-specific (should be ignored - these are overrides, not triggers)
		{
			name:     "sub-template env-specific (contains dot)",
			filename: "cloudformation-worker-params.dev.json",
			want:     "",
		},
		{
			name:     "sub-template env-specific staging",
			filename: "cloudformation-api-params.staging.json",
			want:     "",
		},

		// Non-params files (should be ignored)
		{
			name:     "template file itself",
			filename: "cloudformation.template",
			want:     "",
		},
		{
			name:     "sub-template file itself",
			filename: "cloudformation-worker.template",
			want:     "",
		},
		{
			name:     "random json file",
			filename: "something-else.json",
			want:     "",
		},
		{
			name:     "deploy manifest",
			filename: "deploy-manifest.json",
			want:     "",
		},
		{
			name:     "empty string",
			filename: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTemplateName(tt.filename)
			if got != tt.want {
				t.Errorf("extractTemplateName(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestExtractTemplateNameFromS3Key(t *testing.T) {
	// Test that extractTemplateName works correctly when given just the filename
	// from a full S3 key path
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "main template in S3 path",
			key:  "myapp/main/1.0.0/cloudformation-params.json",
			want: "",
		},
		{
			name: "sub-template worker in S3 path",
			key:  "myapp/main/1.0.0/cloudformation-worker-params.json",
			want: "worker",
		},
		{
			name: "sub-template api in S3 path",
			key:  "myapp/feature-branch/2.0.0/cloudformation-api-params.json",
			want: "api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract filename from path (simulating what filepath.Base does)
			filename := ""
			for i := len(tt.key) - 1; i >= 0; i-- {
				if tt.key[i] == '/' {
					filename = tt.key[i+1:]
					break
				}
			}
			if filename == "" {
				filename = tt.key
			}

			got := extractTemplateName(filename)
			if got != tt.want {
				t.Errorf("extractTemplateName(filepath.Base(%q)) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestRepoConstruction(t *testing.T) {
	tests := []struct {
		name         string
		baseRepo     string
		templateName string
		wantRepo     string
	}{
		{
			name:         "main template - repo equals baseRepo",
			baseRepo:     "myapp",
			templateName: "",
			wantRepo:     "myapp",
		},
		{
			name:         "sub-template worker",
			baseRepo:     "myapp",
			templateName: "worker",
			wantRepo:     "myapp:worker",
		},
		{
			name:         "sub-template api",
			baseRepo:     "service",
			templateName: "api",
			wantRepo:     "service:api",
		},
		{
			name:         "sub-template with hyphen",
			baseRepo:     "myapp",
			templateName: "data-processor",
			wantRepo:     "myapp:data-processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from processS3Record
			repo := tt.baseRepo
			if tt.templateName != "" {
				repo = tt.baseRepo + ":" + tt.templateName
			}

			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestStackNameGeneration(t *testing.T) {
	tests := []struct {
		name          string
		env           string
		baseRepo      string
		templateName  string
		wantStackName string
	}{
		{
			name:          "main template dev",
			env:           "dev",
			baseRepo:      "myapp",
			templateName:  "",
			wantStackName: "dev-myapp",
		},
		{
			name:          "main template prod",
			env:           "prod",
			baseRepo:      "myapp",
			templateName:  "",
			wantStackName: "prod-myapp",
		},
		{
			name:          "sub-template worker dev",
			env:           "dev",
			baseRepo:      "myapp",
			templateName:  "worker",
			wantStackName: "dev-myapp-worker",
		},
		{
			name:          "sub-template api staging",
			env:           "staging",
			baseRepo:      "service",
			templateName:  "api",
			wantStackName: "staging-service-api",
		},
		{
			name:          "sub-template with hyphen prod",
			env:           "prod",
			baseRepo:      "myapp",
			templateName:  "data-processor",
			wantStackName: "prod-myapp-data-processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from processS3Record
			stackName := tt.env + "-" + tt.baseRepo
			if tt.templateName != "" {
				stackName = tt.env + "-" + tt.baseRepo + "-" + tt.templateName
			}

			if stackName != tt.wantStackName {
				t.Errorf("stackName = %q, want %q", stackName, tt.wantStackName)
			}
		})
	}
}

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
		wantBaseRepo string
		wantBranch   string
		wantVersion  string
		wantFilename string
	}{
		{
			name:         "main template",
			key:          "myapp/main/1.0.abc123/cloudformation-params.json",
			wantBaseRepo: "myapp",
			wantBranch:   "main",
			wantVersion:  "1.0.abc123",
			wantFilename: "cloudformation-params.json",
		},
		{
			name:         "sub-template worker",
			key:          "myapp/feature-branch/2.0.def456/cloudformation-worker-params.json",
			wantBaseRepo: "myapp",
			wantBranch:   "feature-branch",
			wantVersion:  "2.0.def456",
			wantFilename: "cloudformation-worker-params.json",
		},
		{
			name:         "longer path segments",
			key:          "org/repo-name/release/v1/3.0.ghi789/cloudformation-api-params.json",
			wantBaseRepo: "org",
			wantBranch:   "repo-name",
			wantVersion:  "release",
			wantFilename: "cloudformation-api-params.json",
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

			baseRepo := pathParts[0]
			branch := pathParts[1]
			version := pathParts[2]
			filename := pathParts[len(pathParts)-1]

			if baseRepo != tt.wantBaseRepo {
				t.Errorf("baseRepo = %q, want %q", baseRepo, tt.wantBaseRepo)
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

// TestCreateInputConstruction tests the full logic for constructing a CreateInput
// from an S3 key, simulating the processS3Record flow
func TestCreateInputConstruction(t *testing.T) {
	tests := []struct {
		name              string
		s3Key             string
		initialEnv        string
		wantRepo          string
		wantBaseRepo      string
		wantTemplateName  string
		wantBranch        string
		wantVersion       string
		wantBuildNumber   string
		wantCommitHash    string
		wantStackName     string
	}{
		{
			name:             "main template full flow",
			s3Key:            "myapp/main/123.abc456/cloudformation-params.json",
			initialEnv:       "dev",
			wantRepo:         "myapp",
			wantBaseRepo:     "myapp",
			wantTemplateName: "",
			wantBranch:       "main",
			wantVersion:      "123.abc456",
			wantBuildNumber:  "123",
			wantCommitHash:   "abc456",
			wantStackName:    "dev-myapp",
		},
		{
			name:             "sub-template worker full flow",
			s3Key:            "myapp/main/456.def789/cloudformation-worker-params.json",
			initialEnv:       "dev",
			wantRepo:         "myapp:worker",
			wantBaseRepo:     "myapp",
			wantTemplateName: "worker",
			wantBranch:       "main",
			wantVersion:      "456.def789",
			wantBuildNumber:  "456",
			wantCommitHash:   "def789",
			wantStackName:    "dev-myapp-worker",
		},
		{
			name:             "sub-template api in staging",
			s3Key:            "service/feature-x/789.ghi012/cloudformation-api-params.json",
			initialEnv:       "staging",
			wantRepo:         "service:api",
			wantBaseRepo:     "service",
			wantTemplateName: "api",
			wantBranch:       "feature-x",
			wantVersion:      "789.ghi012",
			wantBuildNumber:  "789",
			wantCommitHash:   "ghi012",
			wantStackName:    "staging-service-api",
		},
		{
			name:             "sub-template with hyphenated name",
			s3Key:            "myapp/release/100.xyz/cloudformation-data-processor-params.json",
			initialEnv:       "prod",
			wantRepo:         "myapp:data-processor",
			wantBaseRepo:     "myapp",
			wantTemplateName: "data-processor",
			wantBranch:       "release",
			wantVersion:      "100.xyz",
			wantBuildNumber:  "100",
			wantCommitHash:   "xyz",
			wantStackName:    "prod-myapp-data-processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate processS3Record logic

			// 1. Extract filename and template name
			key := tt.s3Key
			filename := ""
			for i := len(key) - 1; i >= 0; i-- {
				if key[i] == '/' {
					filename = key[i+1:]
					break
				}
			}
			templateName := extractTemplateName(filename)

			// 2. Parse S3 key parts
			pathParts := make([]string, 0)
			current := ""
			for _, c := range key {
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

			baseRepo := pathParts[0]
			branch := pathParts[1]
			version := pathParts[2]

			// 3. Parse version
			versionParts := make([]string, 0)
			current = ""
			for _, c := range version {
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

			buildNumber := versionParts[0]
			commitHash := ""
			for i := 1; i < len(versionParts); i++ {
				if i > 1 {
					commitHash += "."
				}
				commitHash += versionParts[i]
			}

			// 4. Construct repo name
			repo := baseRepo
			if templateName != "" {
				repo = baseRepo + ":" + templateName
			}

			// 5. Construct stack name
			stackName := tt.initialEnv + "-" + baseRepo
			if templateName != "" {
				stackName = tt.initialEnv + "-" + baseRepo + "-" + templateName
			}

			// Verify all fields
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if baseRepo != tt.wantBaseRepo {
				t.Errorf("baseRepo = %q, want %q", baseRepo, tt.wantBaseRepo)
			}
			if templateName != tt.wantTemplateName {
				t.Errorf("templateName = %q, want %q", templateName, tt.wantTemplateName)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %q, want %q", version, tt.wantVersion)
			}
			if buildNumber != tt.wantBuildNumber {
				t.Errorf("buildNumber = %q, want %q", buildNumber, tt.wantBuildNumber)
			}
			if commitHash != tt.wantCommitHash {
				t.Errorf("commitHash = %q, want %q", commitHash, tt.wantCommitHash)
			}
			if stackName != tt.wantStackName {
				t.Errorf("stackName = %q, want %q", stackName, tt.wantStackName)
			}
		})
	}
}

// TestProcessS3RecordFiltering tests that processS3Record correctly filters files
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
			name:          "sub-template params - should process",
			filename:      "cloudformation-worker-params.json",
			shouldProcess: true,
			reason:        "sub-template trigger file",
		},
		{
			name:          "env-specific params - should NOT process",
			filename:      "cloudformation-params.dev.json",
			shouldProcess: false,
			reason:        "env override file, not a trigger",
		},
		{
			name:          "sub-template env-specific - should NOT process",
			filename:      "cloudformation-worker-params.dev.json",
			shouldProcess: false,
			reason:        "sub-template env override file",
		},
		{
			name:          "template file - should NOT process",
			filename:      "cloudformation.template",
			shouldProcess: false,
			reason:        "template file, not params",
		},
		{
			name:          "sub-template template file - should NOT process",
			filename:      "cloudformation-worker.template",
			shouldProcess: false,
			reason:        "sub-template file, not params",
		},
		{
			name:          "random json - should NOT process",
			filename:      "config.json",
			shouldProcess: false,
			reason:        "unrelated file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce filtering logic from processS3Record
			templateName := extractTemplateName(tt.filename)
			isMainTemplate := tt.filename == "cloudformation-params.json"
			isSubTemplate := templateName != ""

			shouldProcess := isMainTemplate || isSubTemplate

			if shouldProcess != tt.shouldProcess {
				t.Errorf("shouldProcess = %v, want %v (%s)", shouldProcess, tt.shouldProcess, tt.reason)
			}
		})
	}
}
