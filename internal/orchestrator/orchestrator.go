package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
)

// StepFunctionInput represents the input payload for Step Functions executions
type StepFunctionInput struct {
	Repo         string `json:"repo"`                    // Repository name (includes template suffix for sub-templates, e.g., "myapp:worker")
	Env          string `json:"env"`                     // Environment name (dev, staging, prod)
	Branch       string `json:"branch"`                  // Git branch
	Version      string `json:"version"`                 // Version string
	SK           string `json:"sk"`                      // KSUID - DynamoDB sort key
	CommitHash   string `json:"commit_hash"`             // Git commit hash
	S3Bucket     string `json:"s3_bucket"`               // S3 bucket containing artifacts
	S3Key        string `json:"s3_key"`                  // S3 key prefix for artifacts
	TemplateName string `json:"template_name,omitempty"` // Template name for sub-templates (empty for main template)
	BaseRepo     string `json:"base_repo,omitempty"`     // Original repo name without template suffix
}

// Orchestrator manages Step Functions execution lifecycle
type Orchestrator struct {
	sfnClient       *sfn.Client
	stateMachineArn string
	dao             *builddao.DAO
}

// New creates a new Orchestrator instance
func New(sfnClient *sfn.Client, stateMachineArn string, dao *builddao.DAO) *Orchestrator {
	return &Orchestrator{
		sfnClient:       sfnClient,
		stateMachineArn: stateMachineArn,
		dao:             dao,
	}
}

// StartExecution starts a Step Functions execution and atomically updates the build record
// to IN_PROGRESS status with the execution ARN
func (o *Orchestrator) StartExecution(ctx context.Context, input StepFunctionInput) (string, error) {
	// Marshal input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal step function input: %w", err)
	}

	// Generate execution name - replace colons with dashes as Step Functions doesn't allow colons
	// e.g., "myapp:worker" becomes "myapp-worker" in the execution name
	safeRepo := strings.ReplaceAll(input.Repo, ":", "-")
	executionName := fmt.Sprintf("%s-%s-%s", safeRepo, input.Env, input.SK)

	// Start Step Functions execution
	result, err := o.sfnClient.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: aws.String(o.stateMachineArn),
		Name:            aws.String(executionName),
		Input:           aws.String(string(inputJSON)),
	})
	if err != nil {
		return "", fmt.Errorf("failed to start step function execution: %w", err)
	}

	executionArn := aws.ToString(result.ExecutionArn)

	// Atomically update build status to IN_PROGRESS and set execution ARN
	pk := builddao.NewPK(input.Repo, input.Env)
	if err := o.dao.StartExecution(ctx, pk, input.SK, executionArn); err != nil {
		return "", fmt.Errorf("failed to update build status: %w", err)
	}

	return executionArn, nil
}
