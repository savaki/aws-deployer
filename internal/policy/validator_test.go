package policy

import (
	"encoding/json"
	"testing"
)

func TestValidator_ValidateTemplate(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name             string
		template         string
		env              string
		repo             string
		expectAllow      bool
		expectViolations []string
	}{
		{
			name: "Valid template with Lambda and properly named DynamoDB",
			template: `{
				"Resources": {
					"MyFunction": {
						"Type": "AWS::Lambda::Function",
						"Properties": {
							"FunctionName": "my-function"
						}
					},
					"MyFunctionUrl": {
						"Type": "AWS::Lambda::Url",
						"Properties": {
							"TargetFunctionArn": {"Ref": "MyFunction"}
						}
					},
					"MyTable": {
						"Type": "AWS::DynamoDB::Table",
						"Properties": {
							"TableName": "dev-myrepo--users"
						}
					},
					"MyRole": {
						"Type": "AWS::IAM::Role",
						"Properties": {
							"AssumeRolePolicyDocument": {}
						}
					}
				}
			}`,
			env:              "dev",
			repo:             "myrepo",
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "Valid template with only Lambda resources",
			template: `{
				"Resources": {
					"MyFunction": {
						"Type": "AWS::Lambda::Function",
						"Properties": {
							"FunctionName": "my-function"
						}
					},
					"MyFunctionUrl": {
						"Type": "AWS::Lambda::Url",
						"Properties": {
							"TargetFunctionArn": {"Ref": "MyFunction"}
						}
					}
				}
			}`,
			env:              "prod",
			repo:             "testapp",
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "Valid template with Lambda EventSourceMapping",
			template: `{
				"Resources": {
					"MyFunction": {
						"Type": "AWS::Lambda::Function",
						"Properties": {
							"FunctionName": "my-function"
						}
					},
					"MyEventSourceMapping": {
						"Type": "AWS::Lambda::EventSourceMapping",
						"Properties": {
							"EventSourceArn": "arn:aws:sqs:us-east-1:123456789012:my-queue",
							"FunctionName": {"Ref": "MyFunction"}
						}
					}
				}
			}`,
			env:              "dev",
			repo:             "myapp",
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "Valid template with S3, SNS, SQS, and SecretsManager",
			template: `{
				"Resources": {
					"MyBucket": {
						"Type": "AWS::S3::Bucket",
						"Properties": {
							"BucketName": "my-bucket"
						}
					},
					"MyTopic": {
						"Type": "AWS::SNS::Topic",
						"Properties": {
							"TopicName": "my-topic"
						}
					},
					"MySubscription": {
						"Type": "AWS::SNS::Subscription",
						"Properties": {
							"Protocol": "sqs",
							"TopicArn": {"Ref": "MyTopic"}
						}
					},
					"MyTopicPolicy": {
						"Type": "AWS::SNS::TopicPolicy",
						"Properties": {
							"Topics": [{"Ref": "MyTopic"}]
						}
					},
					"MyQueue": {
						"Type": "AWS::SQS::Queue",
						"Properties": {
							"QueueName": "my-queue"
						}
					},
					"MyQueuePolicy": {
						"Type": "AWS::SQS::QueuePolicy",
						"Properties": {
							"Queues": [{"Ref": "MyQueue"}]
						}
					},
					"MySecret": {
						"Type": "AWS::SecretsManager::Secret",
						"Properties": {
							"Name": "my-secret"
						}
					}
				}
			}`,
			env:              "dev",
			repo:             "myrepo",
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "Invalid resource type",
			template: `{
				"Resources": {
					"MyInstance": {
						"Type": "AWS::EC2::Instance",
						"Properties": {
							"InstanceType": "t2.micro"
						}
					}
				}
			}`,
			env:              "dev",
			repo:             "myrepo",
			expectAllow:      false,
			expectViolations: []string{"Resource type 'AWS::EC2::Instance' is not allowed"},
		},
		{
			name: "DynamoDB table with incorrect naming",
			template: `{
				"Resources": {
					"MyTable": {
						"Type": "AWS::DynamoDB::Table",
						"Properties": {
							"TableName": "wrong-name"
						}
					}
				}
			}`,
			env:              "dev",
			repo:             "myrepo",
			expectAllow:      false,
			expectViolations: []string{"DynamoDB table 'wrong-name' must be prefixed with 'dev-myrepo--'"},
		},
		{
			name: "DynamoDB table with correct naming for different environment",
			template: `{
				"Resources": {
					"MyTable": {
						"Type": "AWS::DynamoDB::Table",
						"Properties": {
							"TableName": "prod-webapp--sessions"
						}
					}
				}
			}`,
			env:              "prod",
			repo:             "webapp",
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "DynamoDB table missing TableName property",
			template: `{
				"Resources": {
					"MyTable": {
						"Type": "AWS::DynamoDB::Table",
						"Properties": {}
					}
				}
			}`,
			env:              "dev",
			repo:             "myrepo",
			expectAllow:      false,
			expectViolations: []string{"DynamoDB table 'MISSING_TABLE_NAME' must be prefixed with 'dev-myrepo--'"},
		},
		{
			name: "Multiple violations",
			template: `{
				"Resources": {
					"MyTable": {
						"Type": "AWS::DynamoDB::Table",
						"Properties": {
							"TableName": "wrong-name"
						}
					},
					"InvalidResource1": {
						"Type": "AWS::EC2::Instance",
						"Properties": {}
					},
					"InvalidResource2": {
						"Type": "AWS::RDS::DBInstance",
						"Properties": {}
					}
				}
			}`,
			env:         "dev",
			repo:        "myrepo",
			expectAllow: false,
			expectViolations: []string{
				"Resource type 'AWS::EC2::Instance' is not allowed",
				"Resource type 'AWS::RDS::DBInstance' is not allowed",
				"DynamoDB table 'wrong-name' must be prefixed with 'dev-myrepo--'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var template map[string]interface{}
			err := json.Unmarshal([]byte(tt.template), &template)
			if err != nil {
				t.Fatalf("Failed to parse template JSON: %v", err)
			}

			result, err := validator.ValidateTemplate(template, tt.env, tt.repo)
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if result.Allowed != tt.expectAllow {
				t.Errorf("Expected allowed=%v, got allowed=%v", tt.expectAllow, result.Allowed)
			}

			if tt.expectViolations == nil && len(result.Violations) > 0 {
				t.Errorf("Expected no violations, got: %v", result.Violations)
			}

			if tt.expectViolations != nil {
				if len(result.Violations) == 0 {
					t.Errorf("Expected violations %v, got none", tt.expectViolations)
				} else {
					// Check that all expected violations are present
					violationMap := make(map[string]bool)
					for _, v := range result.Violations {
						violationMap[v] = true
					}

					for _, expected := range tt.expectViolations {
						if !violationMap[expected] {
							t.Errorf("Expected violation '%s' not found in %v", expected, result.Violations)
						}
					}
				}
			}
		})
	}
}

func TestValidator_AllowedResourceTypes(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	allowedTypes := []string{
		"AWS::Lambda::Function",
		"AWS::Lambda::EventSourceMapping",
		"AWS::Lambda::Url",
		"AWS::DynamoDB::Table",
		"AWS::IAM::Role",
		"AWS::IAM::Policy",
		"AWS::Logs::LogGroup",
		"AWS::S3::Bucket",
		"AWS::SNS::Topic",
		"AWS::SNS::Subscription",
		"AWS::SNS::TopicPolicy",
		"AWS::SQS::Queue",
		"AWS::SQS::QueuePolicy",
		"AWS::SecretsManager::Secret",
	}

	for _, resourceType := range allowedTypes {
		t.Run("Allow_"+resourceType, func(t *testing.T) {
			properties := map[string]interface{}{}

			// For DynamoDB tables, provide a properly named table
			if resourceType == "AWS::DynamoDB::Table" {
				properties["TableName"] = "dev-testrepo--test"
			}

			template := map[string]interface{}{
				"Resources": map[string]interface{}{
					"TestResource": map[string]interface{}{
						"Type":       resourceType,
						"Properties": properties,
					},
				},
			}

			result, err := validator.ValidateTemplate(template, "dev", "testrepo")
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if !result.Allowed {
				t.Errorf("Resource type %s should be allowed, but got violations: %v", resourceType, result.Violations)
			}
		})
	}
}

func TestValidator_DynamoDBNamingRules(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name        string
		tableName   string
		env         string
		repo        string
		expectAllow bool
	}{
		{"Valid dev naming", "dev-myapp--users", "dev", "myapp", true},
		{"Valid prod naming", "prod-webapp--sessions", "prod", "webapp", true},
		{"Valid staging naming", "staging-api--cache", "staging", "api", true},
		{"Invalid prefix", "wrong-myapp--users", "dev", "myapp", false},
		{"Missing double dash", "dev-myapp-users", "dev", "myapp", false},
		{"Wrong repo", "dev-wrongrepo--users", "dev", "myapp", false},
		{"Wrong env", "prod-myapp--users", "dev", "myapp", false},
		{"No prefix", "users", "dev", "myapp", false},
		{"Empty table name", "", "dev", "myapp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := map[string]interface{}{
				"Resources": map[string]interface{}{
					"TestTable": map[string]interface{}{
						"Type": "AWS::DynamoDB::Table",
						"Properties": map[string]interface{}{
							"TableName": tt.tableName,
						},
					},
				},
			}

			result, err := validator.ValidateTemplate(template, tt.env, tt.repo)
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if result.Allowed != tt.expectAllow {
				t.Errorf("Table name '%s' with env='%s' repo='%s': expected allowed=%v, got allowed=%v. Violations: %v",
					tt.tableName, tt.env, tt.repo, tt.expectAllow, result.Allowed, result.Violations)
			}
		})
	}
}

func TestValidator_IAMPolicyActions(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name             string
		actions          []string
		expectAllow      bool
		expectViolations []string
	}{
		{
			name:             "Valid logs actions",
			actions:          []string{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid SQS actions",
			actions:          []string{"sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid SNS actions",
			actions:          []string{"sns:Publish", "sns:Subscribe", "sns:Unsubscribe"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid DynamoDB actions",
			actions:          []string{"dynamodb:PutItem", "dynamodb:GetItem", "dynamodb:Query"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid XRay actions",
			actions:          []string{"xray:PutTraceSegments", "xray:PutTelemetryRecords"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid S3 actions",
			actions:          []string{"s3:GetObject", "s3:PutObject", "s3:ListBucket"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid SecretsManager actions",
			actions:          []string{"secretsmanager:GetSecretValue", "secretsmanager:CreateSecret"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Mixed valid actions",
			actions:          []string{"logs:PutLogEvents", "dynamodb:Query", "s3:GetObject", "secretsmanager:GetSecretValue"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Invalid EC2 action",
			actions:          []string{"ec2:RunInstances"},
			expectAllow:      false,
			expectViolations: []string{"IAM Policy contains unauthorized action: 'ec2:RunInstances'. Only log:*, sqs:*, sns:*, dynamodb:*, xray:*, s3:*, and secretsmanager:* actions are allowed."},
		},
		{
			name:             "Invalid Lambda action",
			actions:          []string{"lambda:InvokeFunction"},
			expectAllow:      false,
			expectViolations: []string{"IAM Policy contains unauthorized action: 'lambda:InvokeFunction'. Only log:*, sqs:*, sns:*, dynamodb:*, xray:*, s3:*, and secretsmanager:* actions are allowed."},
		},
		{
			name:        "Mixed valid and invalid actions",
			actions:     []string{"logs:PutLogEvents", "ec2:TerminateInstances", "s3:GetObject"},
			expectAllow: false,
			expectViolations: []string{
				"IAM Policy contains unauthorized action: 'ec2:TerminateInstances'. Only log:*, sqs:*, sns:*, dynamodb:*, xray:*, s3:*, and secretsmanager:* actions are allowed.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := map[string]interface{}{
				"Resources": map[string]interface{}{
					"TestRole": map[string]interface{}{
						"Type": "AWS::IAM::Role",
						"Properties": map[string]interface{}{
							"AssumeRolePolicyDocument": map[string]interface{}{},
						},
					},
					"TestPolicy": map[string]interface{}{
						"Type": "AWS::IAM::Policy",
						"Properties": map[string]interface{}{
							"PolicyName": "TestPolicy",
							"Roles":      []interface{}{"TestRole"},
							"PolicyDocument": map[string]interface{}{
								"Version": "2012-10-17",
								"Statement": []interface{}{
									map[string]interface{}{
										"Effect":   "Allow",
										"Action":   tt.actions,
										"Resource": "*",
									},
								},
							},
						},
					},
				},
			}

			result, err := validator.ValidateTemplate(template, "dev", "testrepo")
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if result.Allowed != tt.expectAllow {
				t.Errorf("Expected allowed=%v, got allowed=%v. Violations: %v", tt.expectAllow, result.Allowed, result.Violations)
			}

			if tt.expectViolations == nil && len(result.Violations) > 0 {
				t.Errorf("Expected no violations, got: %v", result.Violations)
			}

			if tt.expectViolations != nil {
				if len(result.Violations) == 0 {
					t.Errorf("Expected violations %v, got none", tt.expectViolations)
				} else {
					// Check that all expected violations are present
					violationMap := make(map[string]bool)
					for _, v := range result.Violations {
						violationMap[v] = true
					}

					for _, expected := range tt.expectViolations {
						if !violationMap[expected] {
							t.Errorf("Expected violation '%s' not found in %v", expected, result.Violations)
						}
					}
				}
			}
		})
	}
}

func TestValidator_IAMManagedPolicies(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name             string
		managedPolicies  []string
		expectAllow      bool
		expectViolations []string
	}{
		{
			name:             "Valid Lambda basic execution role",
			managedPolicies:  []string{"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid Lambda VPC execution role",
			managedPolicies:  []string{"arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Valid XRay daemon write access",
			managedPolicies:  []string{"arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name: "Multiple valid managed policies",
			managedPolicies: []string{
				"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
				"arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess",
			},
			expectAllow:      true,
			expectViolations: nil,
		},
		{
			name:             "Invalid managed policy",
			managedPolicies:  []string{"arn:aws:iam::aws:policy/AdministratorAccess"},
			expectAllow:      false,
			expectViolations: []string{"IAM Role contains unauthorized managed policy ARN: 'arn:aws:iam::aws:policy/AdministratorAccess'. Only Lambda execution policies are allowed."},
		},
		{
			name: "Mixed valid and invalid managed policies",
			managedPolicies: []string{
				"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
				"arn:aws:iam::aws:policy/PowerUserAccess",
			},
			expectAllow: false,
			expectViolations: []string{
				"IAM Role contains unauthorized managed policy ARN: 'arn:aws:iam::aws:policy/PowerUserAccess'. Only Lambda execution policies are allowed.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := map[string]interface{}{
				"Resources": map[string]interface{}{
					"TestRole": map[string]interface{}{
						"Type": "AWS::IAM::Role",
						"Properties": map[string]interface{}{
							"AssumeRolePolicyDocument": map[string]interface{}{},
							"ManagedPolicyArns":        tt.managedPolicies,
						},
					},
				},
			}

			result, err := validator.ValidateTemplate(template, "dev", "testrepo")
			if err != nil {
				t.Fatalf("Validation failed with error: %v", err)
			}

			if result.Allowed != tt.expectAllow {
				t.Errorf("Expected allowed=%v, got allowed=%v. Violations: %v", tt.expectAllow, result.Allowed, result.Violations)
			}

			if tt.expectViolations == nil && len(result.Violations) > 0 {
				t.Errorf("Expected no violations, got: %v", result.Violations)
			}

			if tt.expectViolations != nil {
				if len(result.Violations) == 0 {
					t.Errorf("Expected violations %v, got none", tt.expectViolations)
				} else {
					// Check that all expected violations are present
					violationMap := make(map[string]bool)
					for _, v := range result.Violations {
						violationMap[v] = true
					}

					for _, expected := range tt.expectViolations {
						if !violationMap[expected] {
							t.Errorf("Expected violation '%s' not found in %v", expected, result.Violations)
						}
					}
				}
			}
		})
	}
}
