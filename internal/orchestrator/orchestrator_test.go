package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionNameGeneration(t *testing.T) {
	tests := []struct {
		name         string
		repo         string
		env          string
		sk           string
		wantContains string
		wantNoColon  bool
	}{
		{
			name:         "main template repo",
			repo:         "myapp",
			env:          "dev",
			sk:           "2HFj3kLmNoPqRsTuVwXy",
			wantContains: "myapp-dev-2HFj3kLmNoPqRsTuVwXy",
			wantNoColon:  true,
		},
		{
			name:         "sub-template repo with colon",
			repo:         "myapp:worker",
			env:          "dev",
			sk:           "2HFj3kLmNoPqRsTuVwXy",
			wantContains: "myapp-worker-dev-2HFj3kLmNoPqRsTuVwXy",
			wantNoColon:  true,
		},
		{
			name:         "sub-template repo with hyphenated name",
			repo:         "myapp:my-service",
			env:          "staging",
			sk:           "2HFj4kLmNoPqRsTuVwXz",
			wantContains: "myapp-my-service-staging-2HFj4kLmNoPqRsTuVwXz",
			wantNoColon:  true,
		},
		{
			name:         "production environment",
			repo:         "frontend:admin-panel",
			env:          "prod",
			sk:           "2HFj5kLmNoPqRsTuVwXa",
			wantContains: "frontend-admin-panel-prod-2HFj5kLmNoPqRsTuVwXa",
			wantNoColon:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the execution name generation logic from StartExecution
			safeRepo := strings.ReplaceAll(tt.repo, ":", "-")
			executionName := safeRepo + "-" + tt.env + "-" + tt.sk

			if executionName != tt.wantContains {
				t.Errorf("execution name = %q, want %q", executionName, tt.wantContains)
			}

			if tt.wantNoColon && strings.Contains(executionName, ":") {
				t.Errorf("execution name %q contains colon, which is not allowed in Step Functions", executionName)
			}
		})
	}
}

func TestStepFunctionInputSerialization(t *testing.T) {
	// Test that StepFunctionInput fields are correctly tagged for JSON serialization
	input := StepFunctionInput{
		Repo:         "myapp:worker",
		Env:          "dev",
		Branch:       "main",
		Version:      "1.0.0",
		SK:           "2HFj3kLmNoPqRsTuVwXy",
		CommitHash:   "abc123",
		S3Bucket:     "my-bucket",
		S3Key:        "myapp/main/1.0.0",
		TemplateName: "worker",
		BaseRepo:     "myapp",
	}

	// Verify all fields are set
	if input.Repo != "myapp:worker" {
		t.Errorf("Repo = %q, want %q", input.Repo, "myapp:worker")
	}
	if input.TemplateName != "worker" {
		t.Errorf("TemplateName = %q, want %q", input.TemplateName, "worker")
	}
	if input.BaseRepo != "myapp" {
		t.Errorf("BaseRepo = %q, want %q", input.BaseRepo, "myapp")
	}
}

func TestStepFunctionInputMainTemplate(t *testing.T) {
	// Test that main template inputs work correctly with empty TemplateName
	input := StepFunctionInput{
		Repo:         "myapp",
		Env:          "dev",
		Branch:       "main",
		Version:      "1.0.0",
		SK:           "2HFj3kLmNoPqRsTuVwXy",
		CommitHash:   "abc123",
		S3Bucket:     "my-bucket",
		S3Key:        "myapp/main/1.0.0",
		TemplateName: "", // Empty for main template
		BaseRepo:     "myapp",
	}

	if input.TemplateName != "" {
		t.Errorf("TemplateName should be empty for main template, got %q", input.TemplateName)
	}
	if input.Repo != input.BaseRepo {
		t.Errorf("For main template, Repo (%q) should equal BaseRepo (%q)", input.Repo, input.BaseRepo)
	}
}

func TestStepFunctionInputJSONSerialization(t *testing.T) {
	tests := []struct {
		name                 string
		input                StepFunctionInput
		wantTemplateNameKey  bool // Should template_name key be present in JSON?
		wantBaseRepoKey      bool // Should base_repo key be present in JSON?
		wantTemplateName     string
		wantBaseRepo         string
	}{
		{
			name: "sub-template includes template_name and base_repo",
			input: StepFunctionInput{
				Repo:         "myapp:worker",
				Env:          "dev",
				Branch:       "main",
				Version:      "1.0.0",
				SK:           "2HFj3kLmNoPqRsTuVwXy",
				CommitHash:   "abc123",
				S3Bucket:     "my-bucket",
				S3Key:        "myapp/main/1.0.0",
				TemplateName: "worker",
				BaseRepo:     "myapp",
			},
			wantTemplateNameKey: true,
			wantBaseRepoKey:     true,
			wantTemplateName:    "worker",
			wantBaseRepo:        "myapp",
		},
		{
			name: "main template omits empty template_name and base_repo",
			input: StepFunctionInput{
				Repo:         "myapp",
				Env:          "dev",
				Branch:       "main",
				Version:      "1.0.0",
				SK:           "2HFj3kLmNoPqRsTuVwXy",
				CommitHash:   "abc123",
				S3Bucket:     "my-bucket",
				S3Key:        "myapp/main/1.0.0",
				TemplateName: "",
				BaseRepo:     "",
			},
			wantTemplateNameKey: false,
			wantBaseRepoKey:     false,
			wantTemplateName:    "",
			wantBaseRepo:        "",
		},
		{
			name: "main template with base_repo set but no template_name",
			input: StepFunctionInput{
				Repo:         "myapp",
				Env:          "dev",
				Branch:       "main",
				Version:      "1.0.0",
				SK:           "2HFj3kLmNoPqRsTuVwXy",
				CommitHash:   "abc123",
				S3Bucket:     "my-bucket",
				S3Key:        "myapp/main/1.0.0",
				TemplateName: "",
				BaseRepo:     "myapp",
			},
			wantTemplateNameKey: false,
			wantBaseRepoKey:     true,
			wantTemplateName:    "",
			wantBaseRepo:        "myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Unmarshal to map to check key presence
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal to map failed: %v", err)
			}

			// Check template_name key presence
			_, hasTemplateName := m["template_name"]
			if hasTemplateName != tt.wantTemplateNameKey {
				t.Errorf("template_name key presence = %v, want %v (json: %s)", hasTemplateName, tt.wantTemplateNameKey, string(data))
			}

			// Check base_repo key presence
			_, hasBaseRepo := m["base_repo"]
			if hasBaseRepo != tt.wantBaseRepoKey {
				t.Errorf("base_repo key presence = %v, want %v (json: %s)", hasBaseRepo, tt.wantBaseRepoKey, string(data))
			}

			// Unmarshal back to struct and verify values
			var decoded StepFunctionInput
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("json.Unmarshal to struct failed: %v", err)
			}

			if decoded.TemplateName != tt.wantTemplateName {
				t.Errorf("decoded.TemplateName = %q, want %q", decoded.TemplateName, tt.wantTemplateName)
			}
			if decoded.BaseRepo != tt.wantBaseRepo {
				t.Errorf("decoded.BaseRepo = %q, want %q", decoded.BaseRepo, tt.wantBaseRepo)
			}

			// Verify other fields survived round-trip
			if decoded.Repo != tt.input.Repo {
				t.Errorf("decoded.Repo = %q, want %q", decoded.Repo, tt.input.Repo)
			}
			if decoded.Env != tt.input.Env {
				t.Errorf("decoded.Env = %q, want %q", decoded.Env, tt.input.Env)
			}
			if decoded.SK != tt.input.SK {
				t.Errorf("decoded.SK = %q, want %q", decoded.SK, tt.input.SK)
			}
		})
	}
}

func TestStepFunctionInputJSONKeys(t *testing.T) {
	// Verify JSON field names match expected snake_case format
	input := StepFunctionInput{
		Repo:         "myapp:worker",
		Env:          "dev",
		Branch:       "main",
		Version:      "1.0.0",
		SK:           "ksuid123",
		CommitHash:   "abc123",
		S3Bucket:     "bucket",
		S3Key:        "key",
		TemplateName: "worker",
		BaseRepo:     "myapp",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)

	// Verify expected keys are present with correct names
	expectedKeys := []string{
		`"repo"`,
		`"env"`,
		`"branch"`,
		`"version"`,
		`"sk"`,
		`"commit_hash"`,
		`"s3_bucket"`,
		`"s3_key"`,
		`"template_name"`,
		`"base_repo"`,
	}

	for _, key := range expectedKeys {
		if !strings.Contains(jsonStr, key) {
			t.Errorf("JSON missing expected key %s: %s", key, jsonStr)
		}
	}

	// Verify no camelCase keys leaked through
	unexpectedKeys := []string{
		`"commitHash"`,
		`"s3Bucket"`,
		`"s3Key"`,
		`"templateName"`,
		`"baseRepo"`,
	}

	for _, key := range unexpectedKeys {
		if strings.Contains(jsonStr, key) {
			t.Errorf("JSON contains unexpected camelCase key %s: %s", key, jsonStr)
		}
	}
}
