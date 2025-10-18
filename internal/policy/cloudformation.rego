package cloudformation

import rego.v1

default allow := false

allow if {
	validate_resources
	validate_dynamodb_naming
	validate_iam_roles
	validate_iam_policies
}

validate_resources if {
	every resource in input.Resources {
		allowed_resource_type(resource.Type)
	}
}

allowed_resource_type(resource_type) if {
	resource_type in {
        "AWS::ApiGatewayV2::Api",
        "AWS::ApiGatewayV2::ApiMapping",
        "AWS::ApiGatewayV2::DomainName",
        "AWS::ApiGatewayV2::Integration",
        "AWS::ApiGatewayV2::Route",
        "AWS::ApiGatewayV2::Stage",
        "AWS::CloudFront::CachePolicy",
        "AWS::CloudFront::Distribution",
        "AWS::CloudFront::OriginRequestPolicy",
        "AWS::DynamoDB::Table",
        "AWS::IAM::Policy",
        "AWS::IAM::Role",
        "AWS::Lambda::EventSourceMapping",
        "AWS::Lambda::Function",
        "AWS::Lambda::Permission",
        "AWS::Lambda::Url",
        "AWS::Lambda::Version",
        "AWS::Logs::LogGroup",
        "AWS::Route53::RecordSet",
        "AWS::S3::Bucket",
        "AWS::SecretsManager::Secret",
        "AWS::SNS::Subscription",
        "AWS::SNS::Topic",
        "AWS::SNS::TopicPolicy",
        "AWS::SQS::Queue",
        "AWS::SQS::QueuePolicy",
        "AWS::StepFunctions::StateMachine"
	}
}

validate_dynamodb_naming if {
	not has_invalid_dynamodb_tables
}

has_invalid_dynamodb_tables if {
	some resource in input.Resources
	resource.Type == "AWS::DynamoDB::Table"
	not valid_dynamodb_name(resource)
}

valid_dynamodb_name(resource) if {
	resource.Type == "AWS::DynamoDB::Table"
	table_name := resource.Properties.TableName
	startswith(table_name, sprintf("%s-%s--", [data.env, data.repo]))
}

violations contains msg if {
	some resource in input.Resources
	not allowed_resource_type(resource.Type)
	msg := sprintf("Resource type '%s' is not allowed", [resource.Type])
}

violations contains msg if {
	some resource in input.Resources
	resource.Type == "AWS::DynamoDB::Table"
	not valid_dynamodb_name(resource)
	msg := sprintf("DynamoDB table '%s' must be prefixed with '%s-%s--'", [
		object.get(resource.Properties, "TableName", "MISSING_TABLE_NAME"),
		data.env,
		data.repo
	])
}

# IAM Role validation
validate_iam_roles if {
	not has_invalid_iam_roles
}

has_invalid_iam_roles if {
	some resource in input.Resources
	resource.Type == "AWS::IAM::Role"
	not valid_iam_role(resource)
}

valid_iam_role(resource) if {
	resource.Type == "AWS::IAM::Role"
	valid_managed_policy_arns(resource)
}

valid_managed_policy_arns(resource) if {
	resource.Type == "AWS::IAM::Role"
	managed_policies := object.get(resource.Properties, "ManagedPolicyArns", [])
	every policy_arn in managed_policies {
		allowed_managed_policy_arn(policy_arn)
	}
}

allowed_managed_policy_arn(policy_arn) if {
	policy_arn in {
		"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
		"arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole",
		"arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
	}
}

# IAM Policy validation
validate_iam_policies if {
	not has_invalid_iam_policies
}

has_invalid_iam_policies if {
	some resource in input.Resources
	resource.Type == "AWS::IAM::Policy"
	not valid_iam_policy(resource)
}

valid_iam_policy(resource) if {
	resource.Type == "AWS::IAM::Policy"
	policy_doc := object.get(resource.Properties, "PolicyDocument", {})
	statements := object.get(policy_doc, "Statement", [])
	every statement in statements {
		valid_policy_statement(statement)
	}
}

valid_policy_statement(statement) if {
	statement.Effect == "Allow"
	actions := statement.Action
	every action in actions {
		allowed_policy_action(action)
	}
}

allowed_policy_action(action) if {
	startswith(action, "logs:")
}

allowed_policy_action(action) if {
	startswith(action, "sqs:")
}

allowed_policy_action(action) if {
	startswith(action, "sns:")
}

allowed_policy_action(action) if {
	startswith(action, "dynamodb:")
}

allowed_policy_action(action) if {
	startswith(action, "xray:")
}

allowed_policy_action(action) if {
	startswith(action, "s3:")
}

allowed_policy_action(action) if {
	startswith(action, "secretsmanager:")
}

# IAM Role violations
violations contains msg if {
	some resource in input.Resources
	resource.Type == "AWS::IAM::Role"
	managed_policies := object.get(resource.Properties, "ManagedPolicyArns", [])
	some policy_arn in managed_policies
	not allowed_managed_policy_arn(policy_arn)
	msg := sprintf("IAM Role contains unauthorized managed policy ARN: '%s'. Only Lambda execution policies are allowed.", [policy_arn])
}

# IAM Policy violations  
violations contains msg if {
	some resource in input.Resources
	resource.Type == "AWS::IAM::Policy"
	policy_doc := object.get(resource.Properties, "PolicyDocument", {})
	statements := object.get(policy_doc, "Statement", [])
	some statement in statements
	statement.Effect == "Allow"
	actions := statement.Action
	some action in actions
	not allowed_policy_action(action)
	msg := sprintf("IAM Policy contains unauthorized action: '%s'. Only log:*, sqs:*, sns:*, dynamodb:*, xray:*, s3:*, and secretsmanager:* actions are allowed.", [action])
}