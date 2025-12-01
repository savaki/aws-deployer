.PHONY: build build-cli clean deploy test frontend-schema frontend-codegen frontend-build frontend frontend-dev

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build parameters
BINARY_NAME=bootstrap
BUILD_DIR=build
LAMBDA_FUNCTIONS=s3-trigger trigger-build deploy-cloudformation check-stack-status update-build-status promote-images server rotator
MULTI_ACCOUNT_FUNCTIONS=acquire-lock fetch-targets initialize-deployments create-stackset deploy-stack-instances check-stackset-status aggregate-results release-lock

# AWS parameters
AWS_REGION ?= us-east-1
ENV ?= dev
S3_BUCKET ?= lmvtfy-github-artifacts

# Optional parameters for custom domain (Route53 + API Gateway)
ZONE_ID ?=
DOMAIN_NAME ?=
CERTIFICATE_ARN ?=

# Optional authorization parameter
ALLOWED_EMAIL ?=

# Optional rotation schedule parameter (days between rotations)
ROTATION_SCHEDULE_DAYS ?=

# Optional deployment mode parameter
DEPLOYMENT_MODE ?=

# Version management - generate once and reuse
VERSION_FILE := .version
VERSION ?= $(shell if [ -f $(VERSION_FILE) ]; then cat $(VERSION_FILE); else date +%Y%m%d-%H%M%S | tee $(VERSION_FILE); fi)

.DEFAULT_GOAL := help

all: build

build: clean
	@echo "Building Lambda functions..."
	@mkdir -p $(BUILD_DIR)
	
	# Build S3 trigger function
	@echo "Building s3-trigger..."
	@cd internal/lambda/s3-trigger && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../$(BUILD_DIR)/s3-trigger/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/s3-trigger && zip -r ../s3-trigger.zip .

	# Build trigger-build function
	@echo "Building trigger-build..."
	@cd internal/lambda/trigger-build && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../$(BUILD_DIR)/trigger-build/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/trigger-build && zip -r ../trigger-build.zip .

	# Build step function Lambda functions
	@echo "Building deploy-cloudformation..."
	@cd internal/lambda/step-functions/deploy-cloudformation && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../$(BUILD_DIR)/deploy-cloudformation/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/deploy-cloudformation && zip -r ../deploy-cloudformation.zip .

	@echo "Building check-stack-status..."
	@cd internal/lambda/step-functions/check-stack-status && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../$(BUILD_DIR)/check-stack-status/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/check-stack-status && zip -r ../check-stack-status.zip .

	@echo "Building update-build-status..."
	@cd internal/lambda/step-functions/update-build-status && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../$(BUILD_DIR)/update-build-status/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/update-build-status && zip -r ../update-build-status.zip .

	@echo "Building promote-images..."
	@cd internal/lambda/step-functions/promote-images && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../$(BUILD_DIR)/promote-images/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/promote-images && zip -r ../promote-images.zip .

	@echo "Building server..."
	@cd internal/lambda/server && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../$(BUILD_DIR)/server/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/server && zip -r ../server.zip .

	@echo "Building rotator..."
	@cd internal/lambda/rotator && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../$(BUILD_DIR)/rotator/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/rotator && zip -r ../rotator.zip .

	# Build multi-account Lambda functions
	@echo "Building acquire-lock..."
	@cd internal/lambda/step-functions/multi-account/acquire-lock && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/acquire-lock/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/acquire-lock && zip -r ../acquire-lock.zip .

	@echo "Building fetch-targets..."
	@cd internal/lambda/step-functions/multi-account/fetch-targets && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/fetch-targets/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/fetch-targets && zip -r ../fetch-targets.zip .

	@echo "Building initialize-deployments..."
	@cd internal/lambda/step-functions/multi-account/initialize-deployments && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/initialize-deployments/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/initialize-deployments && zip -r ../initialize-deployments.zip .

	@echo "Building create-stackset..."
	@cd internal/lambda/step-functions/multi-account/create-stackset && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/create-stackset/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/create-stackset && zip -r ../create-stackset.zip .

	@echo "Building deploy-stack-instances..."
	@cd internal/lambda/step-functions/multi-account/deploy-stack-instances && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/deploy-stack-instances/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/deploy-stack-instances && zip -r ../deploy-stack-instances.zip .

	@echo "Building check-stackset-status..."
	@cd internal/lambda/step-functions/multi-account/check-stackset-status && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/check-stackset-status/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/check-stackset-status && zip -r ../check-stackset-status.zip .

	@echo "Building aggregate-results..."
	@cd internal/lambda/step-functions/multi-account/aggregate-results && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/aggregate-results/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/aggregate-results && zip -r ../aggregate-results.zip .

	@echo "Building release-lock..."
	@cd internal/lambda/step-functions/multi-account/release-lock && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="-s -w" -o ../../../../../$(BUILD_DIR)/release-lock/$(BINARY_NAME) .
	@cd $(BUILD_DIR)/release-lock && zip -r ../release-lock.zip .

	@echo "Build completed successfully!"

clean:
	@echo "Cleaning build directory and version file..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(VERSION_FILE)
	@$(GOCLEAN)

clean-version:
	@echo "Cleaning version file..."
	@rm -f $(VERSION_FILE)

clean-all: clean clean-version
	@echo "All clean targets completed!"

show-version:
	@echo "Current version: $(VERSION)"

test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

bench:
	@echo "Running benchmarks..."
	@./run-benchmarks.sh

deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

upload-to-s3: build
	@echo "Uploading Lambda packages to S3..."
	@echo "Version: $(VERSION)"
	@aws s3 sync $(BUILD_DIR) s3://$(S3_BUCKET)/aws-deployer/$(VERSION)/ \
		--exclude "*" \
		--include "*.zip" \
		--region $(AWS_REGION) \
		--only-show-errors
	@echo "Upload completed!"

deploy-infrastructure: upload-to-s3
	@echo "Deploying infrastructure..."
	$(eval PARAMS := Env=$(ENV) S3BucketName=$(S3_BUCKET) Version=$(VERSION))
	$(if $(ZONE_ID),$(eval PARAMS := $(PARAMS) ZoneId=$(ZONE_ID)))
	$(if $(DOMAIN_NAME),$(eval PARAMS := $(PARAMS) DomainName=$(DOMAIN_NAME)))
	$(if $(CERTIFICATE_ARN),$(eval PARAMS := $(PARAMS) CertificateArn=$(CERTIFICATE_ARN)))
	$(if $(ALLOWED_EMAIL),$(eval PARAMS := $(PARAMS) AllowedEmail=$(ALLOWED_EMAIL)))
	$(if $(ROTATION_SCHEDULE_DAYS),$(eval PARAMS := $(PARAMS) RotationScheduleDays=$(ROTATION_SCHEDULE_DAYS)))
	$(if $(DEPLOYMENT_MODE),$(eval PARAMS := $(PARAMS) DeploymentMode=$(DEPLOYMENT_MODE)))
	@aws cloudformation deploy \
		--template-file cloudformation.template \
		--stack-name $(ENV)-aws-deployer \
		--parameter-overrides $(PARAMS) \
		--capabilities CAPABILITY_NAMED_IAM \
		--region $(AWS_REGION)

update-lambda-code: upload-to-s3
	@echo "Updating Lambda function code from S3..."
	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-s3-trigger \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/s3-trigger.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-trigger-build \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/trigger-build.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-deploy-cloudformation \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/deploy-cloudformation.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-check-stack-status \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/check-stack-status.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-update-build-status \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/update-build-status.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-server \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/server.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-rotator \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/rotator.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-promote-images \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/promote-images.zip \
		--region $(AWS_REGION)

	@aws lambda update-function-code \
		--function-name $(ENV)-aws-deployer-promote-images-multi \
		--s3-bucket $(S3_BUCKET) \
		--s3-key aws-deployer/$(VERSION)/promote-images.zip \
		--region $(AWS_REGION) 2>/dev/null || true

deploy: deploy-infrastructure configure-s3-notification
	@echo "Deployment completed!"

configure-s3-notification:
	@echo "Configuring S3 bucket notification..."
	@LAMBDA_ARN=$$(aws lambda get-function --function-name $(ENV)-aws-deployer-s3-trigger --query 'Configuration.FunctionArn' --output text --region $(AWS_REGION)); \
	sed "s/LAMBDA_FUNCTION_ARN_PLACEHOLDER/$$LAMBDA_ARN/g" s3-notification.json > s3-notification-configured.json
	@aws s3api put-bucket-notification-configuration \
		--bucket $(S3_BUCKET) \
		--notification-configuration file://s3-notification-configured.json \
		--region $(AWS_REGION)
	@rm s3-notification-configured.json
	@echo "S3 bucket notification configured successfully!"

logs-s3-trigger:
	@aws logs tail /aws/lambda/$(ENV)-aws-deployer-s3-trigger --follow --region $(AWS_REGION)

logs-step-function:
	@aws stepfunctions list-executions --state-machine-arn $$(aws cloudformation describe-stacks --stack-name $(ENV)-aws-deployer --query 'Stacks[0].Outputs[?OutputKey==`StateMachineArn`].OutputValue' --output text --region $(AWS_REGION)) --region $(AWS_REGION)

describe-stack:
	@aws cloudformation describe-stacks --stack-name $(ENV)-aws-deployer --region $(AWS_REGION)

build-cli:
	@echo "Building aws-deployer CLI..."
	@cd cmd/aws-deployer && $(GOBUILD) -o aws-deployer .
	@echo "CLI built successfully at cmd/aws-deployer/aws-deployer"
	@echo ""
	@echo "Usage:"
	@echo "  ./cmd/aws-deployer/aws-deployer --help"
	@echo ""
	@echo "Or run without building:"
	@echo "  go run ./cmd/aws-deployer --help"
	@echo "  go run ./cmd/aws-deployer setup-aws --help"
	@echo "  go run ./cmd/aws-deployer setup-github --help"
	@echo "  go run ./cmd/aws-deployer targets --help"

help:
	@echo "Available targets:"
	@echo "  build                    - Build all Lambda functions"
	@echo "  build-cli                - Build the aws-deployer CLI tool"
	@echo "  clean                    - Clean build directory and version file"
	@echo "  clean-version            - Clean version file (.version)"
	@echo "  clean-all                - Clean build directory and version file"
	@echo "  show-version             - Show current version"
	@echo "  test                     - Run tests"
	@echo "  bench                    - Run benchmarks (requires Docker)"
	@echo "  deps                     - Download and tidy dependencies"
	@echo "  upload-to-s3             - Upload Lambda packages to S3"
	@echo "  deploy-infrastructure    - Deploy CloudFormation infrastructure"
	@echo "  update-lambda-code       - Update Lambda function code from S3"
	@echo "  configure-s3-notification - Configure S3 bucket notification"
	@echo "  deploy                   - Deploy infrastructure and configure S3"
	@echo "  logs-s3-trigger          - Tail S3 trigger Lambda logs"
	@echo "  logs-step-function       - List Step Function executions"
	@echo "  describe-stack           - Describe CloudFormation stack"
	@echo "  help                     - Show this help message"
	@echo ""
	@echo "Environment variables:"
	@echo "  AWS_REGION           - AWS region (default: us-east-1)"
	@echo "  ENV                  - Environment name (default: dev)"
	@echo "  S3_BUCKET            - S3 bucket name (default: lmvtfy-github-artifacts)"
	@echo "  VERSION              - Version for deployment packages (default: YYYYMMDD-HHMMSS)"
	@echo "  ZONE_ID              - Route53 Hosted Zone ID (optional)"
	@echo "  DOMAIN_NAME          - Custom domain name for API Gateway (optional)"
	@echo "  CERTIFICATE_ARN      - ACM certificate ARN for custom domain (optional)"
	@echo "  ALLOWED_EMAIL        - Email address for authorization (optional, if empty all authenticated users allowed)"
	@echo "  ROTATION_SCHEDULE_DAYS - Days between session token rotations (optional, default: 1)"
	@echo "  DEPLOYMENT_MODE      - Deployment mode: single or multi (default: single)"
	@echo ""
	@echo "Version Management:"
	@echo "  - Version is generated once per session and stored in .version file"
	@echo "  - Use 'make clean-version' to generate a new version"
	@echo "  - Override with: VERSION=custom-version make deploy"
	@echo ""
	@echo "Custom Domain Setup:"
	@echo "  - To deploy with a custom domain, provide all three parameters:"
	@echo "    make deploy ZONE_ID=Z1234567890ABC DOMAIN_NAME=api.example.com CERTIFICATE_ARN=arn:aws:acm:..."
	@echo ""
	@echo "Frontend targets:"
	@echo "  frontend-schema          - Copy GraphQL schema from backend to frontend"
	@echo "  frontend-codegen         - Generate TypeScript types from GraphQL schema"
	@echo "  frontend-build           - Build frontend (includes schema copy and codegen)"
	@echo "  frontend                 - Alias for frontend-build"
	@echo "  frontend-dev             - Start frontend development server"

# Copy GraphQL schema from backend to frontend
frontend-schema:
	@echo "Copying GraphQL schema to frontend..."
	@cp internal/gql/schema.graphqls frontend/schema.graphqls
	@echo "Schema copied successfully!"

# Generate TypeScript types from GraphQL schema
frontend-codegen: frontend-schema
	@echo "Generating TypeScript types from GraphQL schema..."
	@cd frontend && npm run codegen
	@echo "TypeScript types generated successfully!"

# Build frontend (includes schema copy and codegen)
frontend-build: frontend-codegen
	@echo "Building frontend..."
	@cd frontend && npm run build
	@echo "Frontend built successfully!"

# Alias for frontend-build
frontend: frontend-build

# Start frontend development server
frontend-dev: frontend-schema frontend-codegen
	@echo "Starting frontend development server..."
	@cd frontend && npm run dev
