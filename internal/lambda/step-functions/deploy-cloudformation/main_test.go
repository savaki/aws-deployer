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

func TestTemplateFileSelection(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		wantTemplate string
		wantParams   string
	}{
		{
			name:         "main template (empty template name)",
			templateName: "",
			wantTemplate: "cloudformation.template",
			wantParams:   "cloudformation-params.json",
		},
		{
			name:         "sub-template worker",
			templateName: "worker",
			wantTemplate: "cloudformation-worker.template",
			wantParams:   "cloudformation-worker-params.json",
		},
		{
			name:         "sub-template api",
			templateName: "api",
			wantTemplate: "cloudformation-api.template",
			wantParams:   "cloudformation-api-params.json",
		},
		{
			name:         "sub-template with hyphen",
			templateName: "data-processor",
			wantTemplate: "cloudformation-data-processor.template",
			wantParams:   "cloudformation-data-processor-params.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from HandleDeployCloudFormation
			templateFile := "cloudformation.template"
			paramsFile := "cloudformation-params.json"
			if tt.templateName != "" {
				templateFile = "cloudformation-" + tt.templateName + ".template"
				paramsFile = "cloudformation-" + tt.templateName + "-params.json"
			}

			if templateFile != tt.wantTemplate {
				t.Errorf("template file = %q, want %q", templateFile, tt.wantTemplate)
			}
			if paramsFile != tt.wantParams {
				t.Errorf("params file = %q, want %q", paramsFile, tt.wantParams)
			}
		})
	}
}

func TestEnvSpecificParamsFilename(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		env          string
		wantFilename string
	}{
		{
			name:         "main template dev",
			templateName: "",
			env:          "dev",
			wantFilename: "cloudformation-params.dev.json",
		},
		{
			name:         "main template prod",
			templateName: "",
			env:          "prod",
			wantFilename: "cloudformation-params.prod.json",
		},
		{
			name:         "sub-template worker dev",
			templateName: "worker",
			env:          "dev",
			wantFilename: "cloudformation-worker-params.dev.json",
		},
		{
			name:         "sub-template api staging",
			templateName: "api",
			env:          "staging",
			wantFilename: "cloudformation-api-params.staging.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from downloadAndParseParams
			var envFilename string
			if tt.templateName != "" {
				envFilename = "cloudformation-" + tt.templateName + "-params." + tt.env + ".json"
			} else {
				envFilename = "cloudformation-params." + tt.env + ".json"
			}

			if envFilename != tt.wantFilename {
				t.Errorf("env filename = %q, want %q", envFilename, tt.wantFilename)
			}
		})
	}
}

func TestStackNameGeneration(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		baseRepo     string
		repo         string
		templateName string
		wantStack    string
	}{
		{
			name:         "main template",
			env:          "dev",
			baseRepo:     "",
			repo:         "myapp",
			templateName: "",
			wantStack:    "dev-myapp",
		},
		{
			name:         "sub-template with baseRepo",
			env:          "dev",
			baseRepo:     "myapp",
			repo:         "myapp:worker",
			templateName: "worker",
			wantStack:    "dev-myapp-worker",
		},
		{
			name:         "sub-template without baseRepo falls back to repo",
			env:          "staging",
			baseRepo:     "",
			repo:         "service",
			templateName: "api",
			wantStack:    "staging-service-api",
		},
		{
			name:         "prod environment",
			env:          "prod",
			baseRepo:     "myapp",
			repo:         "myapp:data-processor",
			templateName: "data-processor",
			wantStack:    "prod-myapp-data-processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic from HandleDeployCloudFormation
			baseRepo := tt.baseRepo
			if baseRepo == "" {
				baseRepo = tt.repo
			}

			var stackName string
			if tt.templateName != "" {
				stackName = tt.env + "-" + baseRepo + "-" + tt.templateName
			} else {
				stackName = tt.env + "-" + baseRepo
			}

			if stackName != tt.wantStack {
				t.Errorf("stack name = %q, want %q", stackName, tt.wantStack)
			}
		})
	}
}
