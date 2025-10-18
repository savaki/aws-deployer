package policy

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestValidTemplateDirectory tests all CloudFormation templates in the valid directory
// Each template should pass policy validation
func TestValidTemplateDirectory(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	validDir := "testdata/valid"
	templates, err := discoverTemplateFiles(validDir)
	if err != nil {
		t.Fatalf("Failed to discover template files in %s: %v", validDir, err)
	}

	if len(templates) == 0 {
		t.Fatalf("No template files found in %s", validDir)
	}

	t.Logf("Found %d valid template files to test", len(templates))

	for _, templatePath := range templates {
		t.Run(filepath.Base(templatePath), func(t *testing.T) {
			// Test with default dev environment and generic repo name
			testTemplateValidation(t, validator, templatePath, "dev", "myapp", true)
		})
	}
}

// TestInvalidTemplateDirectory tests all CloudFormation templates in the invalid directory
// Each template should fail policy validation
func TestInvalidTemplateDirectory(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	invalidDir := "testdata/invalid"
	templates, err := discoverTemplateFiles(invalidDir)
	if err != nil {
		t.Fatalf("Failed to discover template files in %s: %v", invalidDir, err)
	}

	if len(templates) == 0 {
		t.Fatalf("No template files found in %s", invalidDir)
	}

	t.Logf("Found %d invalid template files to test", len(templates))

	for _, templatePath := range templates {
		t.Run(filepath.Base(templatePath), func(t *testing.T) {
			// Test with default dev environment and generic repo name
			testTemplateValidation(t, validator, templatePath, "dev", "myapp", false)
		})
	}
}

// TestValidTemplatesWithDifferentEnvironments demonstrates that templates with hardcoded DynamoDB names
// only work with their intended environment/repo combination
func TestValidTemplatesWithDifferentEnvironments(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	validDir := "testdata/valid"
	templates, err := discoverTemplateFiles(validDir)
	if err != nil {
		t.Fatalf("Failed to discover template files in %s: %v", validDir, err)
	}

	environments := []struct {
		env  string
		repo string
	}{
		{"dev", "myapp"},
		{"staging", "webapp"},
		{"prod", "ecommerce"},
		{"test", "dataservice"},
	}

	for _, template := range templates {
		templateName := filepath.Base(template)

		// Determine if this template has DynamoDB tables (and thus hardcoded names)
		templateContent, err := loadTemplate(template)
		if err != nil {
			t.Fatalf("Failed to load template %s: %v", template, err)
		}

		hasDynamoDB := hasResourceType(templateContent, "AWS::DynamoDB::Table")

		for _, envConfig := range environments {
			testName := templateName + "_" + envConfig.env + "_" + envConfig.repo
			t.Run(testName, func(t *testing.T) {
				// Templates without DynamoDB should pass for all environments
				// Templates with DynamoDB should only pass for dev-myapp (their hardcoded naming)
				shouldPass := !hasDynamoDB || (envConfig.env == "dev" && envConfig.repo == "myapp")
				testTemplateValidation(t, validator, template, envConfig.env, envConfig.repo, shouldPass)
			})
		}
	}
}

// TestSpecificInvalidScenarios tests specific invalid scenarios with detailed assertions
func TestSpecificInvalidScenarios(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	testCases := []struct {
		templateFile       string
		env                string
		repo               string
		expectedViolations []string
	}{
		{
			templateFile: "testdata/invalid/bad-dynamodb-naming.yaml",
			env:          "dev",
			repo:         "myapp",
			expectedViolations: []string{
				"DynamoDB table 'users-table-without-proper-prefix' must be prefixed with 'dev-myapp--'",
			},
		},
		{
			templateFile: "testdata/invalid/wrong-environment-prefix.yaml",
			env:          "dev",
			repo:         "myapp",
			expectedViolations: []string{
				"DynamoDB table 'prod-myapp--users' must be prefixed with 'dev-myapp--'",
			},
		},
		{
			templateFile: "testdata/invalid/missing-table-name.yaml",
			env:          "dev",
			repo:         "myapp",
			expectedViolations: []string{
				"DynamoDB table 'MISSING_TABLE_NAME' must be prefixed with 'dev-myapp--'",
			},
		},
		{
			templateFile: "testdata/invalid/multiple-violations.yaml",
			env:          "dev",
			repo:         "myapp",
			expectedViolations: []string{
				"Resource type 'AWS::EC2::Instance' is not allowed",
				"Resource type 'AWS::RDS::DBCluster' is not allowed",
				"DynamoDB table 'wrong-prefix-table' must be prefixed with 'dev-myapp--'",
			},
		},
	}

	for _, tc := range testCases {
		templateName := filepath.Base(tc.templateFile)
		t.Run(templateName, func(t *testing.T) {
			template, err := loadTemplate(tc.templateFile)
			if err != nil {
				t.Fatalf("Failed to load template %s: %v", tc.templateFile, err)
			}

			result, err := validator.ValidateTemplate(template, tc.env, tc.repo)
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if result.Allowed {
				t.Errorf("Template %s should have failed validation but was allowed", templateName)
			}

			if len(result.Violations) == 0 {
				t.Fatalf("Template %s should have violations but got none", templateName)
			}

			// Check that all expected violations are present
			violationMap := make(map[string]bool)
			for _, v := range result.Violations {
				violationMap[v] = true
			}

			for _, expected := range tc.expectedViolations {
				if !violationMap[expected] {
					t.Errorf("Expected violation '%s' not found in %v", expected, result.Violations)
				}
			}

			t.Logf("Template %s correctly failed with violations: %v", templateName, result.Violations)
		})
	}
}

// discoverTemplateFiles recursively finds all .template and .yaml files in the specified directory
func discoverTemplateFiles(dir string) ([]string, error) {
	var templateFiles []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && (strings.HasSuffix(path, ".template") || strings.HasSuffix(path, ".yaml")) {
			templateFiles = append(templateFiles, path)
		}

		return nil
	})

	return templateFiles, err
}

// testTemplateValidation is a helper function that tests a single template file
func testTemplateValidation(t *testing.T, validator *Validator, templatePath, env, repo string, shouldPass bool) {
	template, err := loadTemplate(templatePath)
	if err != nil {
		t.Fatalf("Failed to load template %s: %v", templatePath, err)
	}

	result, err := validator.ValidateTemplate(template, env, repo)
	if err != nil {
		t.Fatalf("Validation failed with error: %v", err)
	}

	templateName := filepath.Base(templatePath)

	if shouldPass {
		if !result.Allowed {
			t.Errorf("Template %s should have passed validation but failed with violations: %v",
				templateName, result.Violations)
		} else {
			t.Logf("Template %s correctly passed validation for env=%s, repo=%s",
				templateName, env, repo)
		}
	} else {
		if result.Allowed {
			t.Errorf("Template %s should have failed validation but passed", templateName)
		} else {
			t.Logf("Template %s correctly failed validation with violations: %v",
				templateName, result.Violations)
		}
	}
}

// loadTemplate loads and parses a CloudFormation template from a file (supports both JSON and YAML)
func loadTemplate(templatePath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	var template map[string]interface{}

	// Determine file format based on extension
	if strings.HasSuffix(templatePath, ".yaml") || strings.HasSuffix(templatePath, ".yml") {
		err = yaml.Unmarshal(content, &template)
	} else {
		err = json.Unmarshal(content, &template)
	}

	if err != nil {
		return nil, err
	}

	return template, nil
}

// hasResourceType checks if a template contains resources of the specified type
func hasResourceType(template map[string]interface{}, resourceType string) bool {
	resources, ok := template["Resources"].(map[string]interface{})
	if !ok {
		return false
	}

	for _, resource := range resources {
		if resourceMap, ok := resource.(map[string]interface{}); ok {
			if resType, ok := resourceMap["Type"].(string); ok && resType == resourceType {
				return true
			}
		}
	}

	return false
}
