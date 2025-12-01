package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/constants"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

// S3Getter abstracts S3 GetObject operations for testing
type S3Getter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// ECRClient defines the ECR operations needed for image promotion
type ECRClient interface {
	BatchGetImage(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error)
	PutImage(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error)
	BatchCheckLayerAvailability(ctx context.Context, params *ecr.BatchCheckLayerAvailabilityInput, optFns ...func(*ecr.Options)) (*ecr.BatchCheckLayerAvailabilityOutput, error)
	GetDownloadUrlForLayer(ctx context.Context, params *ecr.GetDownloadUrlForLayerInput, optFns ...func(*ecr.Options)) (*ecr.GetDownloadUrlForLayerOutput, error)
	InitiateLayerUpload(ctx context.Context, params *ecr.InitiateLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.InitiateLayerUploadOutput, error)
	UploadLayerPart(ctx context.Context, params *ecr.UploadLayerPartInput, optFns ...func(*ecr.Options)) (*ecr.UploadLayerPartOutput, error)
	CompleteLayerUpload(ctx context.Context, params *ecr.CompleteLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.CompleteLayerUploadOutput, error)
}

// ECRClientFactory creates ECR clients for target accounts
type ECRClientFactory interface {
	CreateClient(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error)
}

// HTTPClient abstracts HTTP operations for downloading layers
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DockerManifest represents the common structure of Docker V2 and OCI manifests
type DockerManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	MediaType     string           `json:"mediaType"`
	Config        *ManifestConfig  `json:"config,omitempty"`
	Layers        []ManifestLayer  `json:"layers,omitempty"`
	Manifests     []ManifestLayer  `json:"manifests,omitempty"` // For manifest lists/indexes
	FSLayers      []FSLayer        `json:"fsLayers,omitempty"`  // For V1 schema
}

// ManifestConfig represents the config blob reference
type ManifestConfig struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ManifestLayer represents a layer in the manifest
type ManifestLayer struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// FSLayer represents a layer in V1 schema manifests
type FSLayer struct {
	BlobSum string `json:"blobSum"`
}

// Input represents the Lambda input from Step Functions
type Input struct {
	Env          string `json:"env"`
	Repo         string `json:"repo"`
	SK           string `json:"sk"` // Build KSUID
	S3Bucket     string `json:"s3_bucket"`
	S3Key        string `json:"s3_key"`                  // Prefix like "repo/version/"
	TemplateName string `json:"template_name,omitempty"` // Template name for sub-templates (empty for main template)
	BaseRepo     string `json:"base_repo,omitempty"`     // Original repo name without template suffix

	// For multi-account mode
	TargetAccount string `json:"target_account,omitempty"`
	TargetRegion  string `json:"target_region,omitempty"`
}

// Output represents the Lambda output
type Output struct {
	ImagesPromoted int      `json:"images_promoted"`
	Images         []string `json:"images"` // List of promoted image URIs
	Skipped        bool     `json:"skipped"`
}

// DeployManifest represents the deploy-manifest.json file structure
type DeployManifest struct {
	Images []ImageSpec `json:"images"`
}

// ImageSpec represents a single image to promote
type ImageSpec struct {
	Repository string `json:"repository"` // ECR repo name (e.g., "myapp/api")
	Tag        string `json:"tag"`        // Image tag
}

// Handler handles image promotion
type Handler struct {
	s3Client         S3Getter
	sourceECRClient  ECRClient
	ecrClientFactory ECRClientFactory
	httpClient       HTTPClient
	region           string
}

// DefaultECRClientFactory creates ECR clients using STS role assumption
type DefaultECRClientFactory struct {
	stsClient *sts.Client
	cfg       aws.Config
}

// CreateClient creates an ECR client for the target account/region
func (f *DefaultECRClientFactory) CreateClient(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", targetAccount, constants.ExecutionRoleName)

	creds := stscreds.NewAssumeRoleProvider(f.stsClient, roleARN)
	targetCfg := f.cfg.Copy()
	targetCfg.Credentials = aws.NewCredentialsCache(creds)

	if targetRegion != "" {
		targetCfg.Region = targetRegion
	}

	return ecr.NewFromConfig(targetCfg), nil
}

// NewHandler creates a new Handler
func NewHandler() (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Handler{
		s3Client:        s3.NewFromConfig(cfg),
		sourceECRClient: ecr.NewFromConfig(cfg),
		ecrClientFactory: &DefaultECRClientFactory{
			stsClient: sts.NewFromConfig(cfg),
			cfg:       cfg,
		},
		httpClient: http.DefaultClient,
		region:     cfg.Region,
	}, nil
}

// NewHandlerWithDeps creates a Handler with injected dependencies (for testing)
func NewHandlerWithDeps(s3Client S3Getter, sourceECR ECRClient, factory ECRClientFactory, httpClient HTTPClient, region string) *Handler {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Handler{
		s3Client:         s3Client,
		sourceECRClient:  sourceECR,
		ecrClientFactory: factory,
		httpClient:       httpClient,
		region:           region,
	}
}

// HandlePromoteImages promotes Docker images from source to target ECR
func (h *Handler) HandlePromoteImages(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	// Determine manifest file path
	keyPrefix := strings.TrimRight(input.S3Key, "/")
	manifestKey := fmt.Sprintf("%s/deploy-manifest.json", keyPrefix)

	logger.Info().
		Str("bucket", input.S3Bucket).
		Str("key", manifestKey).
		Str("target_account", input.TargetAccount).
		Str("target_region", input.TargetRegion).
		Msg("Checking for deploy manifest")

	// Try to download the manifest
	manifest, found, err := h.downloadManifest(ctx, input.S3Bucket, manifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest: %w", err)
	}

	if !found || len(manifest.Images) == 0 {
		logger.Info().Msg("No deploy manifest found or no images to promote, skipping")
		return &Output{
			ImagesPromoted: 0,
			Skipped:        true,
		}, nil
	}

	logger.Info().
		Int("image_count", len(manifest.Images)).
		Msg("Found images to promote")

	// Get target ECR client (may be cross-account)
	targetECRClient, err := h.getTargetECRClient(ctx, input.TargetAccount, input.TargetRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to create target ECR client: %w", err)
	}

	// Promote each image
	var promotedImages []string
	for _, image := range manifest.Images {
		imageURI, err := h.promoteImage(ctx, image, targetECRClient, input.TargetAccount, input.TargetRegion)
		if err != nil {
			return nil, fmt.Errorf("failed to promote image %s:%s: %w", image.Repository, image.Tag, err)
		}
		promotedImages = append(promotedImages, imageURI)
		logger.Info().
			Str("repository", image.Repository).
			Str("tag", image.Tag).
			Str("image_uri", imageURI).
			Msg("Successfully promoted image")
	}

	return &Output{
		ImagesPromoted: len(promotedImages),
		Images:         promotedImages,
		Skipped:        false,
	}, nil
}

// downloadManifest downloads and parses deploy-manifest.json from S3
func (h *Handler) downloadManifest(ctx context.Context, bucket, key string) (*DeployManifest, bool, error) {
	result, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *s3types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, false, nil
		}
		// Also check for 404-like errors
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404") {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest DeployManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, false, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	return &manifest, true, nil
}

// getTargetECRClient returns an ECR client for the target account/region
func (h *Handler) getTargetECRClient(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
	// If no target account specified, use the source ECR client
	if targetAccount == "" {
		return h.sourceECRClient, nil
	}

	// Use the factory to create a cross-account client
	return h.ecrClientFactory.CreateClient(ctx, targetAccount, targetRegion)
}

// promoteImage promotes a single image from source to target ECR
func (h *Handler) promoteImage(ctx context.Context, image ImageSpec, targetECR ECRClient, targetAccount, targetRegion string) (string, error) {
	logger := zerolog.Ctx(ctx)

	// Validate image spec
	if image.Repository == "" {
		return "", fmt.Errorf("image repository cannot be empty")
	}
	if image.Tag == "" {
		return "", fmt.Errorf("image tag cannot be empty")
	}

	// Get image manifest from source ECR
	getImageInput := &ecr.BatchGetImageInput{
		RepositoryName: aws.String(image.Repository),
		ImageIds: []ecrtypes.ImageIdentifier{
			{ImageTag: aws.String(image.Tag)},
		},
	}

	getImageResult, err := h.sourceECRClient.BatchGetImage(ctx, getImageInput)
	if err != nil {
		return "", fmt.Errorf("failed to get source image: %w", err)
	}

	if len(getImageResult.Images) == 0 {
		return "", fmt.Errorf("source image not found: %s:%s", image.Repository, image.Tag)
	}

	sourceImage := getImageResult.Images[0]
	if sourceImage.ImageManifest == nil {
		return "", fmt.Errorf("source image manifest is nil: %s:%s", image.Repository, image.Tag)
	}

	logger.Debug().
		Str("repository", image.Repository).
		Str("tag", image.Tag).
		Msg("Retrieved source image manifest")

	// For cross-account promotion, we need to copy layers first
	if targetAccount != "" {
		if err := h.copyLayers(ctx, image.Repository, *sourceImage.ImageManifest, targetECR); err != nil {
			return "", fmt.Errorf("failed to copy layers: %w", err)
		}
	}

	// Put image to target ECR
	putImageInput := &ecr.PutImageInput{
		RepositoryName: aws.String(image.Repository),
		ImageManifest:  sourceImage.ImageManifest,
		ImageTag:       aws.String(image.Tag),
	}

	// Include manifest media type if available
	if sourceImage.ImageManifestMediaType != nil {
		putImageInput.ImageManifestMediaType = sourceImage.ImageManifestMediaType
	}

	_, err = targetECR.PutImage(ctx, putImageInput)
	if err != nil {
		// Check if image already exists (idempotent)
		var imageExistsErr *ecrtypes.ImageAlreadyExistsException
		if errors.As(err, &imageExistsErr) {
			logger.Info().
				Str("repository", image.Repository).
				Str("tag", image.Tag).
				Msg("Image already exists in target, skipping")
		} else {
			return "", fmt.Errorf("failed to put image to target ECR: %w", err)
		}
	}

	// Construct the target image URI
	var imageURI string
	if targetAccount != "" {
		region := targetRegion
		if region == "" {
			region = h.region
		}
		imageURI = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
			targetAccount, region, image.Repository, image.Tag)
	} else {
		imageURI = fmt.Sprintf("%s:%s", image.Repository, image.Tag)
	}

	return imageURI, nil
}

// copyLayers copies all layers referenced in a manifest from source to target ECR
func (h *Handler) copyLayers(ctx context.Context, repository, manifestJSON string, targetECR ECRClient) error {
	logger := zerolog.Ctx(ctx)

	// Parse the manifest to extract layer digests
	digests, err := extractLayerDigests(manifestJSON)
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	if len(digests) == 0 {
		logger.Debug().Msg("No layers to copy")
		return nil
	}

	logger.Info().
		Int("layer_count", len(digests)).
		Msg("Checking layer availability in target")

	// Check which layers are missing in the target
	missingDigests, err := h.findMissingLayers(ctx, repository, digests, targetECR)
	if err != nil {
		return fmt.Errorf("failed to check layer availability: %w", err)
	}

	if len(missingDigests) == 0 {
		logger.Info().Msg("All layers already exist in target")
		return nil
	}

	logger.Info().
		Int("missing_count", len(missingDigests)).
		Msg("Copying missing layers to target")

	// Copy each missing layer
	for _, digest := range missingDigests {
		if err := h.copyLayer(ctx, repository, digest, targetECR); err != nil {
			return fmt.Errorf("failed to copy layer %s: %w", digest, err)
		}
		logger.Debug().
			Str("digest", digest).
			Msg("Copied layer")
	}

	return nil
}

// extractLayerDigests parses a Docker manifest and returns all blob digests (config + layers)
func extractLayerDigests(manifestJSON string) ([]string, error) {
	var manifest DockerManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	var digests []string

	// Add config digest if present (V2/OCI format)
	if manifest.Config != nil && manifest.Config.Digest != "" {
		digests = append(digests, manifest.Config.Digest)
	}

	// Add layer digests (V2/OCI format)
	for _, layer := range manifest.Layers {
		if layer.Digest != "" {
			digests = append(digests, layer.Digest)
		}
	}

	// Handle V1 schema (fsLayers)
	for _, layer := range manifest.FSLayers {
		if layer.BlobSum != "" {
			digests = append(digests, layer.BlobSum)
		}
	}

	// Handle manifest lists/indexes - these reference other manifests, not layers directly
	// For manifest lists, we would need to recursively fetch each referenced manifest
	// For now, we'll skip manifest lists as they're less common for Lambda-sized containers

	return digests, nil
}

// findMissingLayers checks which layers are missing in the target repository
func (h *Handler) findMissingLayers(ctx context.Context, repository string, digests []string, targetECR ECRClient) ([]string, error) {
	// ECR BatchCheckLayerAvailability has a limit of 100 digests per call
	const batchSize = 100
	var missingDigests []string

	for i := 0; i < len(digests); i += batchSize {
		end := i + batchSize
		if end > len(digests) {
			end = len(digests)
		}
		batch := digests[i:end]

		result, err := targetECR.BatchCheckLayerAvailability(ctx, &ecr.BatchCheckLayerAvailabilityInput{
			RepositoryName: aws.String(repository),
			LayerDigests:   batch,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check layer availability: %w", err)
		}

		// Collect digests of layers that are not available
		for _, layer := range result.Layers {
			if layer.LayerAvailability != ecrtypes.LayerAvailabilityAvailable {
				if layer.LayerDigest != nil {
					missingDigests = append(missingDigests, *layer.LayerDigest)
				}
			}
		}

		// Also check failures (layers that couldn't be checked are assumed missing)
		for _, failure := range result.Failures {
			if failure.LayerDigest != nil {
				missingDigests = append(missingDigests, *failure.LayerDigest)
			}
		}
	}

	return missingDigests, nil
}

// copyLayer copies a single layer from source to target ECR
func (h *Handler) copyLayer(ctx context.Context, repository, digest string, targetECR ECRClient) error {
	// Get download URL from source
	downloadResult, err := h.sourceECRClient.GetDownloadUrlForLayer(ctx, &ecr.GetDownloadUrlForLayerInput{
		RepositoryName: aws.String(repository),
		LayerDigest:    aws.String(digest),
	})
	if err != nil {
		return fmt.Errorf("failed to get download URL: %w", err)
	}

	if downloadResult.DownloadUrl == nil {
		return fmt.Errorf("download URL is nil for layer %s", digest)
	}

	// Download the layer blob
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, *downloadResult.DownloadUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download layer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code downloading layer: %d", resp.StatusCode)
	}

	// Read the entire layer into memory
	// Note: For very large layers, this could be problematic in Lambda (max 10GB memory)
	// In practice, container layers are typically < 1GB
	layerData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read layer data: %w", err)
	}

	// Initiate upload to target
	initResult, err := targetECR.InitiateLayerUpload(ctx, &ecr.InitiateLayerUploadInput{
		RepositoryName: aws.String(repository),
	})
	if err != nil {
		return fmt.Errorf("failed to initiate layer upload: %w", err)
	}

	if initResult.UploadId == nil {
		return fmt.Errorf("upload ID is nil")
	}

	// Upload the layer in parts (ECR requires parts to be between 5MB and 5GB)
	// For simplicity, we'll upload in a single part if < 5GB (which it always will be for Lambda)
	const maxPartSize = 5 * 1024 * 1024 * 1024 // 5GB

	var lastByteUploaded int64 = -1
	partSize := len(layerData)
	if int64(partSize) > maxPartSize {
		partSize = int(maxPartSize)
	}

	for start := 0; start < len(layerData); start += partSize {
		end := start + partSize
		if end > len(layerData) {
			end = len(layerData)
		}

		_, err = targetECR.UploadLayerPart(ctx, &ecr.UploadLayerPartInput{
			RepositoryName: aws.String(repository),
			UploadId:       initResult.UploadId,
			PartFirstByte:  aws.Int64(int64(start)),
			PartLastByte:   aws.Int64(int64(end - 1)),
			LayerPartBlob:  layerData[start:end],
		})
		if err != nil {
			return fmt.Errorf("failed to upload layer part: %w", err)
		}

		lastByteUploaded = int64(end - 1)
	}

	// Complete the upload
	// Calculate the SHA256 digest for verification
	layerDigest := calculateDigest(layerData)

	_, err = targetECR.CompleteLayerUpload(ctx, &ecr.CompleteLayerUploadInput{
		RepositoryName: aws.String(repository),
		UploadId:       initResult.UploadId,
		LayerDigests:   []string{layerDigest},
	})
	if err != nil {
		// Check if layer already exists (race condition with parallel uploads)
		if strings.Contains(err.Error(), "LayerAlreadyExistsException") {
			return nil
		}
		return fmt.Errorf("failed to complete layer upload (uploaded %d bytes): %w", lastByteUploaded+1, err)
	}

	return nil
}

// calculateDigest calculates the sha256 digest of data in Docker's format
func calculateDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

type HandlerFunc func(context.Context, *Input) (*Output, error)

func withLogger(handler HandlerFunc, logger zerolog.Logger) HandlerFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler(ctx, input)
	}
}

func withFailBuildOnError(handler HandlerFunc, build *builddao.DAO) HandlerFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		output, err := handler(ctx, input)
		if err != nil {
			status := builddao.BuildStatusFailed
			updateInput := builddao.UpdateInput{
				PK:       builddao.NewPK(input.Repo, input.Env),
				SK:       input.SK,
				Status:   &status,
				ErrorMsg: aws.String(err.Error()),
			}
			if updateErr := build.UpdateStatus(ctx, updateInput); updateErr != nil {
				zerolog.Ctx(ctx).Error().
					Err(updateErr).
					Stringer("id", builddao.NewID(updateInput.PK, updateInput.SK)).
					Msg("failed to update build status to FAILED")
			}
			return nil, err
		}
		return output, nil
	}
}

func lambdaAction(c *cli.Context) error {
	container, err := di.New(c.String("env"),
		di.WithProviders(
			di.ProvideLogger,
			di.ProvideBuildDAO,
		),
	)
	if err != nil {
		return err
	}

	var (
		logger = di.MustGet[zerolog.Logger](container).With().Str("lambda", "promote-images").Logger()
		build  = di.MustGet[*builddao.DAO](container)
	)

	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	promoteImages := handler.HandlePromoteImages
	promoteImages = withLogger(promoteImages, logger)
	promoteImages = withFailBuildOnError(promoteImages, build)

	lambda.Start(promoteImages)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "promote-images").Logger()

	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	input := &Input{
		Env:           c.String("env"),
		Repo:          c.String("repo"),
		SK:            c.String("build-id"),
		S3Bucket:      c.String("s3-bucket"),
		S3Key:         c.String("s3-key"),
		TargetAccount: c.String("target-account"),
		TargetRegion:  c.String("target-region"),
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandlePromoteImages(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "promote-images",
		Usage:          "Promote Docker images from source to target ECR",
		DefaultCommand: "lambda",
		Commands: []*cli.Command{
			{
				Name:  "lambda",
				Usage: "Start Lambda handler",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment",
						EnvVars:  []string{"ENV"},
						Required: true,
					},
				},
				Action: lambdaAction,
			},
			{
				Name:  "run",
				Usage: "Run locally for testing",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment",
						EnvVars:  []string{"ENV"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "repo",
						Usage:    "Repository name",
						EnvVars:  []string{"REPO"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "build-id",
						Usage:    "Build KSUID",
						EnvVars:  []string{"BUILD_ID"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "s3-bucket",
						Usage:    "S3 bucket name",
						EnvVars:  []string{"S3_BUCKET"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "s3-key",
						Usage:    "S3 key prefix",
						EnvVars:  []string{"S3_KEY"},
						Required: true,
					},
					&cli.StringFlag{
						Name:    "target-account",
						Usage:   "Target AWS account ID (optional, for cross-account)",
						EnvVars: []string{"TARGET_ACCOUNT"},
					},
					&cli.StringFlag{
						Name:    "target-region",
						Usage:   "Target AWS region (optional)",
						EnvVars: []string{"TARGET_REGION"},
					},
				},
				Action: runAction,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
