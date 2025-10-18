package errors

import "errors"

var (
	ErrStateMachineARNRequired  = errors.New("STATE_MACHINE_ARN environment variable is required")
	ErrInvalidS3KeyFormat       = errors.New("invalid S3 key format")
	ErrInvalidVersionFormat     = errors.New("invalid version format")
	ErrStackNotFound            = errors.New("stack not found")
	ErrNoCloudFormationTemplate = errors.New("no CloudFormation template found in S3 path")
)
