package di

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/orchestrator"
	"github.com/savaki/aws-deployer/internal/services"
)

func ProvideAWSConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx)
}

func ProvideDynamoDB(config aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(config)
}

func ProvideStepFunctions(config aws.Config) *sfn.Client {
	return sfn.NewFromConfig(config)
}

func ProvideOrchestrator(sfnClient *sfn.Client, dao *builddao.DAO, config *services.Config) (*orchestrator.Orchestrator, error) {
	// Determine which state machine to use based on deployment mode
	var stateMachineArn string
	if config.DeploymentMode == "multi" {
		stateMachineArn = config.MultiAccountStateMachineArn
		if stateMachineArn == "" {
			return nil, fmt.Errorf("MULTI_ACCOUNT_STATE_MACHINE_ARN required in multi deployment mode")
		}
	} else {
		stateMachineArn = config.StateMachineArn
		if stateMachineArn == "" {
			return nil, fmt.Errorf("STATE_MACHINE_ARN required")
		}
	}

	return orchestrator.New(sfnClient, stateMachineArn, dao), nil
}
