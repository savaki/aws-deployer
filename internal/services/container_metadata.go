package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
)

// ContainerImage represents a single container image in the deployment
type ContainerImage struct {
	Name          string `json:"name"`
	Registry      string `json:"registry"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest,omitempty"`
	Signed        bool   `json:"signed"`
	ParameterName string `json:"parameterName"`
}

// ContainerMetadata contains the list of container images for a deployment
type ContainerMetadata struct {
	Images []ContainerImage `json:"images"`
}

// ContainerMetadataParser handles parsing and validation of container-images.json
type ContainerMetadataParser interface {
	// Parse downloads and parses container-images.json from S3
	Parse(ctx context.Context, s3Bucket, s3Prefix string) (*ContainerMetadata, error)

	// ValidateRegistries checks that all registries are in the allowed list
	ValidateRegistries(ctx context.Context, metadata *ContainerMetadata, allowedRegistries []string) error

	// BuildImageURIs constructs full ECR URIs for all images
	BuildImageURIs(metadata *ContainerMetadata, accountID, region string) map[string]string
}

type containerMetadataParser struct {
	s3Client *s3.Client
	logger   zerolog.Logger
}

// NewContainerMetadataParser creates a new container metadata parser
func NewContainerMetadataParser(
	s3Client *s3.Client,
	logger zerolog.Logger,
) ContainerMetadataParser {
	return &containerMetadataParser{
		s3Client: s3Client,
		logger:   logger.With().Str("service", "container_metadata_parser").Logger(),
	}
}

// Parse downloads and parses container-images.json from S3
func (p *containerMetadataParser) Parse(ctx context.Context, s3Bucket, s3Prefix string) (*ContainerMetadata, error) {
	key := strings.TrimRight(s3Prefix, "/") + "/container-images.json"

	logger := p.logger.With().
		Str("s3_bucket", s3Bucket).
		Str("s3_key", key).
		Logger()

	logger.Info().Msg("downloading container metadata")

	// Download the file from S3
	output, err := p.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to download container-images.json")
		return nil, fmt.Errorf("failed to download container-images.json: %w", err)
	}
	defer output.Body.Close()

	// Read and parse JSON
	data, err := io.ReadAll(output.Body)
	if err != nil {
		logger.Error().Err(err).Msg("failed to read container metadata")
		return nil, fmt.Errorf("failed to read container-images.json: %w", err)
	}

	var metadata ContainerMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		logger.Error().Err(err).Msg("failed to parse container metadata json")
		return nil, fmt.Errorf("failed to parse container-images.json: %w", err)
	}

	logger.Info().
		Int("image_count", len(metadata.Images)).
		Msg("parsed container metadata")

	return &metadata, nil
}

// ValidateRegistries checks that all registries are in the allowed list
func (p *containerMetadataParser) ValidateRegistries(ctx context.Context, metadata *ContainerMetadata, allowedRegistries []string) error {
	logger := p.logger.With().
		Strs("allowed_registries", allowedRegistries).
		Logger()

	logger.Info().Msg("validating registries")

	// Build a map of allowed registries for faster lookup
	allowed := make(map[string]bool)
	for _, registry := range allowedRegistries {
		allowed[registry] = true
	}

	// Check each image's registry
	var invalidRegistries []string
	for _, image := range metadata.Images {
		if !allowed[image.Registry] {
			invalidRegistries = append(invalidRegistries, image.Registry)
			logger.Warn().
				Str("registry", image.Registry).
				Str("image_name", image.Name).
				Msg("registry not in allowed list")
		}
	}

	if len(invalidRegistries) > 0 {
		return fmt.Errorf("invalid registries found: %v (allowed: %v)", invalidRegistries, allowedRegistries)
	}

	logger.Info().Msg("all registries validated successfully")
	return nil
}

// BuildImageURIs constructs full ECR URIs for all images
// Returns a map of parameter name -> full ECR URI
func (p *containerMetadataParser) BuildImageURIs(metadata *ContainerMetadata, accountID, region string) map[string]string {
	uris := make(map[string]string)

	for _, image := range metadata.Images {
		// Build full ECR URI: {accountId}.dkr.ecr.{region}.amazonaws.com/{registry}:{tag}
		uri := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
			accountID,
			region,
			image.Registry,
			image.Tag,
		)

		// If digest is provided, use it instead of tag (more secure)
		if image.Digest != "" {
			uri = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s@%s",
				accountID,
				region,
				image.Registry,
				image.Digest,
			)
		}

		uris[image.ParameterName] = uri

		p.logger.Debug().
			Str("parameter_name", image.ParameterName).
			Str("image_uri", uri).
			Msg("built image uri")
	}

	return uris
}
