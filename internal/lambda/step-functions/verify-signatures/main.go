package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/signer"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/models"
	"github.com/savaki/aws-deployer/internal/services"
)

type Handler struct {
	verifier       services.SignatureVerifier
	metadataParser services.ContainerMetadataParser
	ssmClient      *ssm.Client
	s3Client       *s3.Client
	logger         zerolog.Logger
	region         string
	accountID      string
}

type VerificationResult struct {
	VerificationPassed  bool     `json:"verificationPassed"`
	LambdasVerified     int      `json:"lambdasVerified"`
	ContainersVerified  int      `json:"containersVerified"`
	Warnings            []string `json:"warnings"`
	Errors              []string `json:"errors"`
	EnforcementMode     string   `json:"enforcementMode"`
	HasContainerImages  bool     `json:"hasContainerImages"`
	VerificationEnabled bool     `json:"verificationEnabled"`
}

func NewHandler(env string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	logger := zerolog.New(os.Stderr).With().
		Timestamp().
		Str("lambda", "verify-signatures").
		Str("env", env).
		Logger()

	s3Client := s3.NewFromConfig(cfg)
	signerClient := signer.NewFromConfig(cfg)
	ssmClient := ssm.NewFromConfig(cfg)

	verifier := services.NewSignatureVerifier(signerClient, s3Client, logger)
	metadataParser := services.NewContainerMetadataParser(s3Client, logger)

	// Get account ID and region from STS
	accountID := os.Getenv("AWS_ACCOUNT_ID")
	if accountID == "" {
		accountID = "000000000000" // Will be overridden by actual account if needed
	}

	return &Handler{
		verifier:       verifier,
		metadataParser: metadataParser,
		ssmClient:      ssmClient,
		s3Client:       s3Client,
		logger:         logger,
		region:         cfg.Region,
		accountID:      accountID,
	}, nil
}

func (h *Handler) HandleVerifySignatures(
	ctx context.Context,
	input *models.StepFunctionInput,
) (result *VerificationResult, err error) {
	logger := h.logger.With().
		Str("repo", input.Repo).
		Str("env", input.Env).
		Str("version", input.Version).
		Logger()

	ctx = logger.WithContext(ctx)

	defer func(begin time.Time) {
		logger.Info().
			Dur("duration_ms", time.Since(begin)).
			Bool("passed", result != nil && result.VerificationPassed).
			Msg("verify signatures completed")
	}(time.Now())

	result = &VerificationResult{
		VerificationPassed: true,
		Warnings:           []string{},
		Errors:             []string{},
		EnforcementMode:    "warn",
	}

	// Step 1: Check if verification is enabled
	logger.Info().Msg("checking if signature verification is enabled")
	enabled, enforcementMode, err := h.getVerificationConfig(ctx, input.Env)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to get verification config, assuming disabled")
		result.VerificationEnabled = false
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to check verification config: %v", err))
		return result, nil
	}

	result.VerificationEnabled = enabled
	result.EnforcementMode = enforcementMode

	if !enabled {
		logger.Info().Msg("signature verification is disabled")
		return result, nil
	}

	logger.Info().
		Str("enforcement_mode", enforcementMode).
		Msg("signature verification is enabled")

	// Step 2: Try to parse container-images.json (optional file)
	prefix := strings.TrimRight(input.S3Key, "/")
	metadata, err := h.metadataParser.Parse(ctx, input.S3Bucket, prefix)
	if err != nil {
		// Check if it's a "not found" error
		var apiErr smithy.APIError
		if ok := err.(smithy.APIError); ok != nil {
			apiErr = ok
		}

		if apiErr != nil && apiErr.ErrorCode() == "NoSuchKey" {
			logger.Info().Msg("no container-images.json found - assuming Lambda-only deployment")
			result.HasContainerImages = false
		} else {
			errMsg := fmt.Sprintf("Failed to parse container-images.json: %v", err)
			logger.Error().Err(err).Msg("failed to parse container metadata")
			result.Errors = append(result.Errors, errMsg)
			result.VerificationPassed = false

			if enforcementMode == "enforce" {
				return result, fmt.Errorf(errMsg)
			}
		}
	} else {
		result.HasContainerImages = true
	}

	// Step 3: Verify container images if present
	if result.HasContainerImages && metadata != nil {
		logger.Info().
			Int("image_count", len(metadata.Images)).
			Msg("verifying container images")

		// Get allowed registries from SSM
		allowedRegistries, err := h.getAllowedRegistries(ctx, input.Env, input.Repo)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to get allowed registries: %v", err)
			logger.Error().Err(err).Msg("failed to get allowed registries")
			result.Errors = append(result.Errors, errMsg)
			result.VerificationPassed = false

			if enforcementMode == "enforce" {
				return result, fmt.Errorf(errMsg)
			}
		} else {
			// Validate registries
			if err := h.metadataParser.ValidateRegistries(ctx, metadata, allowedRegistries); err != nil {
				errMsg := fmt.Sprintf("Registry validation failed: %v", err)
				logger.Error().Err(err).Msg("registry validation failed")
				result.Errors = append(result.Errors, errMsg)
				result.VerificationPassed = false

				if enforcementMode == "enforce" {
					return result, fmt.Errorf(errMsg)
				}
			}

			// Verify each container image signature
			imageURIs := h.metadataParser.BuildImageURIs(metadata, h.accountID, h.region)
			for paramName, imageURI := range imageURIs {
				logger.Info().
					Str("parameter_name", paramName).
					Str("image_uri", imageURI).
					Msg("verifying container image signature")

				verifyResult, err := h.verifier.VerifyContainerSignature(ctx, imageURI)
				if err != nil {
					errMsg := fmt.Sprintf("Failed to verify %s: %v", imageURI, err)
					logger.Error().Err(err).Msg("signature verification failed")
					result.Errors = append(result.Errors, errMsg)
					result.VerificationPassed = false
				} else if !verifyResult.Verified {
					warnMsg := fmt.Sprintf("Container image %s is not signed or signature invalid: %s",
						imageURI, verifyResult.ErrorMessage)
					logger.Warn().Msg(warnMsg)
					result.Warnings = append(result.Warnings, warnMsg)

					if enforcementMode == "enforce" {
						result.Errors = append(result.Errors, warnMsg)
						result.VerificationPassed = false
					}
				} else {
					result.ContainersVerified++
					logger.Info().
						Str("signed_by", verifyResult.SignedBy).
						Msg("container signature verified")
				}
			}
		}
	}

	// Step 4: TODO - Verify Lambda function signatures
	// This would require parsing the CloudFormation template to find Lambda functions
	// and verifying each Lambda zip file in S3
	logger.Info().Msg("lambda signature verification not yet implemented")

	// Final result
	if result.VerificationPassed {
		logger.Info().
			Int("containers_verified", result.ContainersVerified).
			Int("lambdas_verified", result.LambdasVerified).
			Msg("all signatures verified successfully")
	} else if enforcementMode == "warn" {
		logger.Warn().
			Int("error_count", len(result.Errors)).
			Msg("signature verification failed but enforcement mode is warn")
		result.VerificationPassed = true // Allow deployment in warn mode
	}

	return result, nil
}

// getVerificationConfig checks if signature verification is enabled for the environment
func (h *Handler) getVerificationConfig(ctx context.Context, env string) (enabled bool, enforcementMode string, err error) {
	// Check if Lambda verification is enabled
	lambdaVerificationPath := fmt.Sprintf("/%s/aws-deployer/signing/lambda-verification", env)
	output, err := h.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: &lambdaVerificationPath,
	})
	if err != nil {
		// Parameter not found means verification is disabled
		return false, "warn", nil
	}

	enabled = output.Parameter.Value != nil && *output.Parameter.Value == "enabled"

	// Get enforcement mode
	enforcementPath := fmt.Sprintf("/%s/aws-deployer/signing/enforcement-mode", env)
	enforcementOutput, err := h.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: &enforcementPath,
	})
	if err != nil {
		enforcementMode = "warn" // Default to warn mode
	} else if enforcementOutput.Parameter.Value != nil {
		enforcementMode = *enforcementOutput.Parameter.Value
	}

	return enabled, enforcementMode, nil
}

// getAllowedRegistries gets the list of allowed ECR registries for a repo
func (h *Handler) getAllowedRegistries(ctx context.Context, env, repo string) ([]string, error) {
	ssmPath := fmt.Sprintf("/%s/aws-deployer/ecr-registries/%s", env, repo)
	output, err := h.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: &ssmPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get allowed registries from SSM: %w", err)
	}

	if output.Parameter.Value == nil || *output.Parameter.Value == "" {
		return []string{}, nil
	}

	// Parse comma-separated list
	registries := strings.Split(*output.Parameter.Value, ",")

	// Trim whitespace
	for i, reg := range registries {
		registries[i] = strings.TrimSpace(reg)
	}

	return registries, nil
}

func main() {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "dev"
	}

	handler, err := NewHandler(env)
	if err != nil {
		panic(fmt.Sprintf("failed to create handler: %v", err))
	}

	lambda.Start(handler.HandleVerifySignatures)
}
