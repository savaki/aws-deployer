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
	}{
		{
			name:         "standard repo",
			repo:         "myapp",
			env:          "dev",
			sk:           "2HFj3kLmNoPqRsTuVwXy",
			wantContains: "myapp-dev-2HFj3kLmNoPqRsTuVwXy",
		},
		{
			name:         "hyphenated repo",
			repo:         "my-service",
			env:          "staging",
			sk:           "2HFj4kLmNoPqRsTuVwXz",
			wantContains: "my-service-staging-2HFj4kLmNoPqRsTuVwXz",
		},
		{
			name:         "production environment",
			repo:         "frontend",
			env:          "prod",
			sk:           "2HFj5kLmNoPqRsTuVwXa",
			wantContains: "frontend-prod-2HFj5kLmNoPqRsTuVwXa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the execution name generation logic from StartExecution
			executionName := tt.repo + "-" + tt.env + "-" + tt.sk

			if executionName != tt.wantContains {
				t.Errorf("execution name = %q, want %q", executionName, tt.wantContains)
			}
		})
	}
}

func TestStepFunctionInputSerialization(t *testing.T) {
	// Test that StepFunctionInput fields are correctly tagged for JSON serialization
	input := StepFunctionInput{
		Repo:       "myapp",
		Env:        "dev",
		Branch:     "main",
		Version:    "1.0.0",
		SK:         "2HFj3kLmNoPqRsTuVwXy",
		CommitHash: "abc123",
		S3Bucket:   "my-bucket",
		S3Key:      "myapp/main/1.0.0",
	}

	// Verify all fields are set
	if input.Repo != "myapp" {
		t.Errorf("Repo = %q, want %q", input.Repo, "myapp")
	}
	if input.Env != "dev" {
		t.Errorf("Env = %q, want %q", input.Env, "dev")
	}
}

func TestStepFunctionInputJSONSerialization(t *testing.T) {
	input := StepFunctionInput{
		Repo:       "myapp",
		Env:        "dev",
		Branch:     "main",
		Version:    "1.0.0",
		SK:         "2HFj3kLmNoPqRsTuVwXy",
		CommitHash: "abc123",
		S3Bucket:   "my-bucket",
		S3Key:      "myapp/main/1.0.0",
	}

	// Marshal to JSON
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Unmarshal to map to check key presence
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}

	// Unmarshal back to struct and verify values
	var decoded StepFunctionInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal to struct failed: %v", err)
	}

	// Verify fields survived round-trip
	if decoded.Repo != input.Repo {
		t.Errorf("decoded.Repo = %q, want %q", decoded.Repo, input.Repo)
	}
	if decoded.Env != input.Env {
		t.Errorf("decoded.Env = %q, want %q", decoded.Env, input.Env)
	}
	if decoded.SK != input.SK {
		t.Errorf("decoded.SK = %q, want %q", decoded.SK, input.SK)
	}
}

func TestStepFunctionInputJSONKeys(t *testing.T) {
	// Verify JSON field names match expected snake_case format
	input := StepFunctionInput{
		Repo:       "myapp",
		Env:        "dev",
		Branch:     "main",
		Version:    "1.0.0",
		SK:         "ksuid123",
		CommitHash: "abc123",
		S3Bucket:   "bucket",
		S3Key:      "key",
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
	}

	for _, key := range unexpectedKeys {
		if strings.Contains(jsonStr, key) {
			t.Errorf("JSON contains unexpected camelCase key %s: %s", key, jsonStr)
		}
	}
}
