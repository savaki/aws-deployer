package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
)

// Mock implementations

type mockS3Client struct {
	getObjectFunc func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m.getObjectFunc(ctx, params, optFns...)
}

type mockECRClient struct {
	batchGetImageFunc               func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error)
	putImageFunc                    func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error)
	batchCheckLayerAvailabilityFunc func(ctx context.Context, params *ecr.BatchCheckLayerAvailabilityInput, optFns ...func(*ecr.Options)) (*ecr.BatchCheckLayerAvailabilityOutput, error)
	getDownloadUrlForLayerFunc      func(ctx context.Context, params *ecr.GetDownloadUrlForLayerInput, optFns ...func(*ecr.Options)) (*ecr.GetDownloadUrlForLayerOutput, error)
	initiateLayerUploadFunc         func(ctx context.Context, params *ecr.InitiateLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.InitiateLayerUploadOutput, error)
	uploadLayerPartFunc             func(ctx context.Context, params *ecr.UploadLayerPartInput, optFns ...func(*ecr.Options)) (*ecr.UploadLayerPartOutput, error)
	completeLayerUploadFunc         func(ctx context.Context, params *ecr.CompleteLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.CompleteLayerUploadOutput, error)
	createRepositoryFunc            func(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
	describeRepositoriesFunc        func(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
}

func (m *mockECRClient) BatchGetImage(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
	if m.batchGetImageFunc != nil {
		return m.batchGetImageFunc(ctx, params, optFns...)
	}
	return nil, errors.New("batchGetImageFunc not set")
}

func (m *mockECRClient) PutImage(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
	if m.putImageFunc != nil {
		return m.putImageFunc(ctx, params, optFns...)
	}
	return nil, errors.New("putImageFunc not set")
}

func (m *mockECRClient) BatchCheckLayerAvailability(ctx context.Context, params *ecr.BatchCheckLayerAvailabilityInput, optFns ...func(*ecr.Options)) (*ecr.BatchCheckLayerAvailabilityOutput, error) {
	if m.batchCheckLayerAvailabilityFunc != nil {
		return m.batchCheckLayerAvailabilityFunc(ctx, params, optFns...)
	}
	// Default: all layers available
	var layers []ecrtypes.Layer
	for _, digest := range params.LayerDigests {
		layers = append(layers, ecrtypes.Layer{
			LayerDigest:       aws.String(digest),
			LayerAvailability: ecrtypes.LayerAvailabilityAvailable,
		})
	}
	return &ecr.BatchCheckLayerAvailabilityOutput{Layers: layers}, nil
}

func (m *mockECRClient) GetDownloadUrlForLayer(ctx context.Context, params *ecr.GetDownloadUrlForLayerInput, optFns ...func(*ecr.Options)) (*ecr.GetDownloadUrlForLayerOutput, error) {
	if m.getDownloadUrlForLayerFunc != nil {
		return m.getDownloadUrlForLayerFunc(ctx, params, optFns...)
	}
	return nil, errors.New("getDownloadUrlForLayerFunc not set")
}

func (m *mockECRClient) InitiateLayerUpload(ctx context.Context, params *ecr.InitiateLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.InitiateLayerUploadOutput, error) {
	if m.initiateLayerUploadFunc != nil {
		return m.initiateLayerUploadFunc(ctx, params, optFns...)
	}
	return nil, errors.New("initiateLayerUploadFunc not set")
}

func (m *mockECRClient) UploadLayerPart(ctx context.Context, params *ecr.UploadLayerPartInput, optFns ...func(*ecr.Options)) (*ecr.UploadLayerPartOutput, error) {
	if m.uploadLayerPartFunc != nil {
		return m.uploadLayerPartFunc(ctx, params, optFns...)
	}
	return nil, errors.New("uploadLayerPartFunc not set")
}

func (m *mockECRClient) CompleteLayerUpload(ctx context.Context, params *ecr.CompleteLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.CompleteLayerUploadOutput, error) {
	if m.completeLayerUploadFunc != nil {
		return m.completeLayerUploadFunc(ctx, params, optFns...)
	}
	return nil, errors.New("completeLayerUploadFunc not set")
}

func (m *mockECRClient) CreateRepository(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
	if m.createRepositoryFunc != nil {
		return m.createRepositoryFunc(ctx, params, optFns...)
	}
	return nil, errors.New("createRepositoryFunc not set")
}

func (m *mockECRClient) DescribeRepositories(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	if m.describeRepositoriesFunc != nil {
		return m.describeRepositoriesFunc(ctx, params, optFns...)
	}
	// Default: repository exists
	return &ecr.DescribeRepositoriesOutput{
		Repositories: []ecrtypes.Repository{
			{RepositoryName: aws.String(params.RepositoryNames[0])},
		},
	}, nil
}

type mockECRClientFactory struct {
	createClientFunc func(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error)
}

func (m *mockECRClientFactory) CreateClient(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
	return m.createClientFunc(ctx, targetAccount, targetRegion)
}

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

// Helper to create a test context with logger
func testContext() context.Context {
	logger := zerolog.New(io.Discard)
	return logger.WithContext(context.Background())
}

// Helper to create S3 response with container-images.json
func s3ContainerImagesResponse(images ContainerImages) *s3.GetObjectOutput {
	data, _ := json.Marshal(images)
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}
}

// Tests for HandlePromoteImages

func TestHandlePromoteImages_NoManifest(t *testing.T) {
	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, &mockNoSuchKeyError{}
		},
	}

	handler := NewHandlerWithDeps(s3Client, nil, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Skipped {
		t.Error("expected Skipped=true when no manifest")
	}
	if output.ImagesPromoted != 0 {
		t.Errorf("expected ImagesPromoted=0, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_EmptyManifest(t *testing.T) {
	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(ContainerImages{Images: []ContainerImage{}}), nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, nil, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Skipped {
		t.Error("expected Skipped=true for empty manifest")
	}
}

func TestHandlePromoteImages_Success_SingleImage(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{
						ImageManifest:          aws.String(`{"config":{}}`),
						ImageManifestMediaType: aws.String("application/vnd.docker.distribution.manifest.v2+json"),
					},
				},
			}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Skipped {
		t.Error("expected Skipped=false")
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
	if len(output.Images) != 1 {
		t.Errorf("expected 1 image URI, got %d", len(output.Images))
	}
}

func TestHandlePromoteImages_Success_MultipleImages(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
			{Name: "worker", Registry: "myapp/worker", Tag: "1.0.0"},
			{Name: "scheduler", Registry: "myapp/scheduler", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 3 {
		t.Errorf("expected ImagesPromoted=3, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_CrossAccount(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	sourceECRClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
	}

	targetECRClient := &mockECRClient{
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	factory := &mockECRClientFactory{
		createClientFunc: func(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
			if targetAccount != "123456789012" {
				t.Errorf("expected targetAccount=123456789012, got %s", targetAccount)
			}
			if targetRegion != "eu-west-1" {
				t.Errorf("expected targetRegion=eu-west-1, got %s", targetRegion)
			}
			return targetECRClient, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, sourceECRClient, factory, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:           "prod",
		Repo:          "myapp",
		SK:            "abc123",
		S3Bucket:      "bucket",
		S3Key:         "myapp/main/1.0.0",
		TargetAccount: "123456789012",
		TargetRegion:  "eu-west-1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
	// Check that image URI includes target account and target region
	if len(output.Images) > 0 {
		expectedURI := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/myapp/api:1.0.0"
		if output.Images[0] != expectedURI {
			t.Errorf("expected image URI %q, got %q", expectedURI, output.Images[0])
		}
	}
}

func TestHandlePromoteImages_S3Error(t *testing.T) {
	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, errors.New("S3 access denied")
		},
	}

	handler := NewHandlerWithDeps(s3Client, nil, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for S3 failure")
	}
	if !strings.Contains(err.Error(), "failed to download container-images.json") {
		t.Errorf("expected 'failed to download container-images.json' error, got: %v", err)
	}
}

func TestHandlePromoteImages_InvalidManifestJSON(t *testing.T) {
	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader([]byte(`{invalid json}`))),
			}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, nil, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse container-images.json") {
		t.Errorf("expected 'failed to parse container-images.json' error, got: %v", err)
	}
}

func TestHandlePromoteImages_ECRGetImageError(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return nil, errors.New("ECR access denied")
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for ECR failure")
	}
	if !strings.Contains(err.Error(), "failed to get source image") {
		t.Errorf("expected 'failed to get source image' error, got: %v", err)
	}
}

func TestHandlePromoteImages_SourceImageNotFound(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "nonexistent"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{}, // Empty - image not found
			}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for missing source image")
	}
	if !strings.Contains(err.Error(), "source image not found") {
		t.Errorf("expected 'source image not found' error, got: %v", err)
	}
}

func TestHandlePromoteImages_ECRPutImageError(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return nil, errors.New("repository not found")
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for PutImage failure")
	}
	if !strings.Contains(err.Error(), "failed to put image to target ECR") {
		t.Errorf("expected 'failed to put image to target ECR' error, got: %v", err)
	}
}

func TestHandlePromoteImages_ImageAlreadyExists(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return nil, &ecrtypes.ImageAlreadyExistsException{Message: aws.String("image exists")}
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	// Should succeed even when image already exists (idempotent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_FactoryError(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	factory := &mockECRClientFactory{
		createClientFunc: func(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
			return nil, errors.New("failed to assume role")
		},
	}

	handler := NewHandlerWithDeps(s3Client, nil, factory, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:           "prod",
		Repo:          "myapp",
		SK:            "abc123",
		S3Bucket:      "bucket",
		S3Key:         "myapp/main/1.0.0",
		TargetAccount: "123456789012",
	})

	if err == nil {
		t.Fatal("expected error for factory failure")
	}
	if !strings.Contains(err.Error(), "failed to create target ECR client") {
		t.Errorf("expected 'failed to create target ECR client' error, got: %v", err)
	}
}

func TestHandlePromoteImages_EmptyRepository(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "", Tag: "1.0.0"}, // Empty registry
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for empty repository")
	}
	if !strings.Contains(err.Error(), "repository cannot be empty") {
		t.Errorf("expected 'repository cannot be empty' error, got: %v", err)
	}
}

func TestHandlePromoteImages_EmptyTag(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: ""}, // Empty tag
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for empty tag")
	}
	if !strings.Contains(err.Error(), "tag cannot be empty") {
		t.Errorf("expected 'tag cannot be empty' error, got: %v", err)
	}
}

func TestHandlePromoteImages_NilManifest(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: nil}, // Nil manifest
				},
			}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for nil manifest")
	}
	if !strings.Contains(err.Error(), "source image manifest is nil") {
		t.Errorf("expected 'source image manifest is nil' error, got: %v", err)
	}
}

// Mock for NoSuchKey error
type mockNoSuchKeyError struct{}

func (e *mockNoSuchKeyError) Error() string {
	return "NoSuchKey: The specified key does not exist"
}

// Tests for data structures (existing tests, kept for coverage)

func TestContainerImagesParsing(t *testing.T) {
	tests := []struct {
		name       string
		jsonData   string
		wantCount  int
		wantImages []ContainerImage
		wantErr    bool
	}{
		{
			name: "single image",
			jsonData: `{
				"images": [
					{"name": "api", "registry": "myapp/api", "tag": "1.0.0-abc123"}
				]
			}`,
			wantCount: 1,
			wantImages: []ContainerImage{
				{Name: "api", Registry: "myapp/api", Tag: "1.0.0-abc123"},
			},
			wantErr: false,
		},
		{
			name: "multiple images",
			jsonData: `{
				"images": [
					{"name": "api", "registry": "myapp/api", "tag": "1.0.0"},
					{"name": "worker", "registry": "myapp/worker", "tag": "1.0.0"},
					{"name": "scheduler", "registry": "myapp/scheduler", "tag": "1.0.0"}
				]
			}`,
			wantCount: 3,
			wantImages: []ContainerImage{
				{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
				{Name: "worker", Registry: "myapp/worker", Tag: "1.0.0"},
				{Name: "scheduler", Registry: "myapp/scheduler", Tag: "1.0.0"},
			},
			wantErr: false,
		},
		{
			name:       "empty images array",
			jsonData:   `{"images": []}`,
			wantCount:  0,
			wantImages: []ContainerImage{},
			wantErr:    false,
		},
		{
			name:     "invalid json",
			jsonData: `{invalid}`,
			wantErr:  true,
		},
		{
			name:       "missing images field",
			jsonData:   `{}`,
			wantCount:  0,
			wantImages: nil,
			wantErr:    false,
		},
		{
			name: "full container-images.json format",
			jsonData: `{
				"images": [
					{
						"name": "echo",
						"registry": "deployer-test/echo",
						"tag": "14.0d3f85",
						"digest": "sha256:1d8aa5d4ed2f070dd32cfdde7758a394f3051456eeed6668ef468b663c5f8738",
						"signed": false,
						"parameterName": "EchoImageUri"
					}
				]
			}`,
			wantCount: 1,
			wantImages: []ContainerImage{
				{
					Name:          "echo",
					Registry:      "deployer-test/echo",
					Tag:           "14.0d3f85",
					Digest:        "sha256:1d8aa5d4ed2f070dd32cfdde7758a394f3051456eeed6668ef468b663c5f8738",
					Signed:        false,
					ParameterName: "EchoImageUri",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var containerImages ContainerImages
			err := json.Unmarshal([]byte(tt.jsonData), &containerImages)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(containerImages.Images) != tt.wantCount {
				t.Errorf("len(images) = %d, want %d", len(containerImages.Images), tt.wantCount)
			}

			for i, want := range tt.wantImages {
				if i >= len(containerImages.Images) {
					break
				}
				got := containerImages.Images[i]
				if got.Registry != want.Registry {
					t.Errorf("images[%d].Registry = %q, want %q", i, got.Registry, want.Registry)
				}
				if got.Tag != want.Tag {
					t.Errorf("images[%d].Tag = %q, want %q", i, got.Tag, want.Tag)
				}
			}
		})
	}
}

func TestContainerImageToImageSpec(t *testing.T) {
	ci := ContainerImage{
		Name:          "echo",
		Registry:      "deployer-test/echo",
		Tag:           "14.0d3f85",
		Digest:        "sha256:abc123",
		Signed:        false,
		ParameterName: "EchoImageUri",
	}

	spec := ci.ToImageSpec()

	if spec.Repository != "deployer-test/echo" {
		t.Errorf("Repository = %q, want %q", spec.Repository, "deployer-test/echo")
	}
	if spec.Tag != "14.0d3f85" {
		t.Errorf("Tag = %q, want %q", spec.Tag, "14.0d3f85")
	}
}

func TestContainerImagesKeyGeneration(t *testing.T) {
	tests := []struct {
		name        string
		s3Key       string
		wantKeyPath string
	}{
		{
			name:        "simple path",
			s3Key:       "myapp/main/1.2.3",
			wantKeyPath: "myapp/main/1.2.3/container-images.json",
		},
		{
			name:        "path with trailing slash",
			s3Key:       "myapp/main/1.2.3/",
			wantKeyPath: "myapp/main/1.2.3/container-images.json",
		},
		{
			name:        "nested path",
			s3Key:       "org/repo/feature/2.0.0",
			wantKeyPath: "org/repo/feature/2.0.0/container-images.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPrefix := strings.TrimRight(tt.s3Key, "/")
			containerImagesKey := fmt.Sprintf("%s/container-images.json", keyPrefix)

			if containerImagesKey != tt.wantKeyPath {
				t.Errorf("containerImagesKey = %q, want %q", containerImagesKey, tt.wantKeyPath)
			}
		})
	}
}

func TestTargetImageURIGeneration(t *testing.T) {
	tests := []struct {
		name          string
		targetAccount string
		region        string
		repository    string
		tag           string
		wantURI       string
	}{
		{
			name:          "cross-account image URI",
			targetAccount: "123456789012",
			region:        "us-east-1",
			repository:    "myapp/api",
			tag:           "1.0.0",
			wantURI:       "123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp/api:1.0.0",
		},
		{
			name:          "different region",
			targetAccount: "987654321098",
			region:        "eu-west-1",
			repository:    "service/worker",
			tag:           "v2.0.0-abc",
			wantURI:       "987654321098.dkr.ecr.eu-west-1.amazonaws.com/service/worker:v2.0.0-abc",
		},
		{
			name:          "no target account - simple URI",
			targetAccount: "",
			region:        "us-east-1",
			repository:    "myapp/api",
			tag:           "1.0.0",
			wantURI:       "myapp/api:1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imageURI string
			if tt.targetAccount != "" {
				imageURI = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
					tt.targetAccount, tt.region, tt.repository, tt.tag)
			} else {
				imageURI = fmt.Sprintf("%s:%s", tt.repository, tt.tag)
			}

			if imageURI != tt.wantURI {
				t.Errorf("imageURI = %q, want %q", imageURI, tt.wantURI)
			}
		})
	}
}

func TestInputParsing(t *testing.T) {
	tests := []struct {
		name      string
		jsonData  string
		wantInput Input
	}{
		{
			name: "single-account input",
			jsonData: `{
				"env": "dev",
				"repo": "myapp",
				"sk": "abc123",
				"s3_bucket": "artifacts",
				"s3_key": "myapp/main/1.0.0"
			}`,
			wantInput: Input{
				Env:      "dev",
				Repo:     "myapp",
				SK:       "abc123",
				S3Bucket: "artifacts",
				S3Key:    "myapp/main/1.0.0",
			},
		},
		{
			name: "multi-account input",
			jsonData: `{
				"env": "prod",
				"repo": "myapp",
				"sk": "xyz789",
				"s3_bucket": "artifacts",
				"s3_key": "myapp/main/2.0.0",
				"target_account": "123456789012",
				"target_region": "eu-west-1"
			}`,
			wantInput: Input{
				Env:           "prod",
				Repo:          "myapp",
				SK:            "xyz789",
				S3Bucket:      "artifacts",
				S3Key:         "myapp/main/2.0.0",
				TargetAccount: "123456789012",
				TargetRegion:  "eu-west-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input Input
			if err := json.Unmarshal([]byte(tt.jsonData), &input); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if input.Env != tt.wantInput.Env {
				t.Errorf("Env = %q, want %q", input.Env, tt.wantInput.Env)
			}
			if input.Repo != tt.wantInput.Repo {
				t.Errorf("Repo = %q, want %q", input.Repo, tt.wantInput.Repo)
			}
			if input.TargetAccount != tt.wantInput.TargetAccount {
				t.Errorf("TargetAccount = %q, want %q", input.TargetAccount, tt.wantInput.TargetAccount)
			}
		})
	}
}

// Tests for layer copy functionality

func TestExtractLayerDigests(t *testing.T) {
	tests := []struct {
		name         string
		manifestJSON string
		wantDigests  []string
		wantErr      bool
	}{
		{
			name: "V2 manifest with config and layers",
			manifestJSON: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"config": {
					"mediaType": "application/vnd.docker.container.image.v1+json",
					"size": 7023,
					"digest": "sha256:config123"
				},
				"layers": [
					{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"size": 32654,
						"digest": "sha256:layer1"
					},
					{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"size": 16724,
						"digest": "sha256:layer2"
					}
				]
			}`,
			wantDigests: []string{"sha256:config123", "sha256:layer1", "sha256:layer2"},
			wantErr:     false,
		},
		{
			name: "OCI manifest",
			manifestJSON: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"config": {
					"mediaType": "application/vnd.oci.image.config.v1+json",
					"digest": "sha256:ociconfig"
				},
				"layers": [
					{"digest": "sha256:ocilayer1"},
					{"digest": "sha256:ocilayer2"}
				]
			}`,
			wantDigests: []string{"sha256:ociconfig", "sha256:ocilayer1", "sha256:ocilayer2"},
			wantErr:     false,
		},
		{
			name:         "empty manifest",
			manifestJSON: `{}`,
			wantDigests:  nil,
			wantErr:      false,
		},
		{
			name:         "invalid JSON",
			manifestJSON: `{invalid`,
			wantErr:      true,
		},
		{
			name: "V1 schema with fsLayers",
			manifestJSON: `{
				"schemaVersion": 1,
				"fsLayers": [
					{"blobSum": "sha256:v1layer1"},
					{"blobSum": "sha256:v1layer2"}
				]
			}`,
			wantDigests: []string{"sha256:v1layer1", "sha256:v1layer2"},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digests, err := extractLayerDigests(tt.manifestJSON)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(digests) != len(tt.wantDigests) {
				t.Errorf("got %d digests, want %d", len(digests), len(tt.wantDigests))
				return
			}

			for i, want := range tt.wantDigests {
				if digests[i] != want {
					t.Errorf("digests[%d] = %q, want %q", i, digests[i], want)
				}
			}
		})
	}
}

func TestCalculateDigest(t *testing.T) {
	// Test with known data
	data := []byte("hello world")
	got := calculateDigest(data)

	// SHA256 of "hello world" is well-known
	want := "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if got != want {
		t.Errorf("calculateDigest() = %q, want %q", got, want)
	}
}

func TestHandlePromoteImages_CrossAccount_WithLayerCopy(t *testing.T) {
	// Test cross-account promotion with layer copy using streaming
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	// Mock layer data and its real SHA256 digest
	// sha256("test layer content") = 47b353ad39d8e...
	layerData := []byte("test layer content")
	layerDigest := calculateDigest(layerData)

	// Docker manifest with config and layers using the real digest
	dockerManifest := fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {"digest": "%s"},
		"layers": [
			{"digest": "%s"},
			{"digest": "%s"}
		]
	}`, layerDigest, layerDigest, layerDigest)

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	sourceECRClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(dockerManifest)},
				},
			}, nil
		},
		getDownloadUrlForLayerFunc: func(ctx context.Context, params *ecr.GetDownloadUrlForLayerInput, optFns ...func(*ecr.Options)) (*ecr.GetDownloadUrlForLayerOutput, error) {
			return &ecr.GetDownloadUrlForLayerOutput{
				DownloadUrl: aws.String("http://example.com/layer"),
			}, nil
		},
	}

	layerUploadCount := 0
	targetECRClient := &mockECRClient{
		batchCheckLayerAvailabilityFunc: func(ctx context.Context, params *ecr.BatchCheckLayerAvailabilityInput, optFns ...func(*ecr.Options)) (*ecr.BatchCheckLayerAvailabilityOutput, error) {
			// All layers are missing
			var layers []ecrtypes.Layer
			for _, digest := range params.LayerDigests {
				layers = append(layers, ecrtypes.Layer{
					LayerDigest:       aws.String(digest),
					LayerAvailability: ecrtypes.LayerAvailabilityUnavailable,
				})
			}
			return &ecr.BatchCheckLayerAvailabilityOutput{Layers: layers}, nil
		},
		initiateLayerUploadFunc: func(ctx context.Context, params *ecr.InitiateLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.InitiateLayerUploadOutput, error) {
			return &ecr.InitiateLayerUploadOutput{
				UploadId: aws.String("upload-123"),
			}, nil
		},
		uploadLayerPartFunc: func(ctx context.Context, params *ecr.UploadLayerPartInput, optFns ...func(*ecr.Options)) (*ecr.UploadLayerPartOutput, error) {
			return &ecr.UploadLayerPartOutput{}, nil
		},
		completeLayerUploadFunc: func(ctx context.Context, params *ecr.CompleteLayerUploadInput, optFns ...func(*ecr.Options)) (*ecr.CompleteLayerUploadOutput, error) {
			layerUploadCount++
			return &ecr.CompleteLayerUploadOutput{}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	factory := &mockECRClientFactory{
		createClientFunc: func(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
			return targetECRClient, nil
		},
	}

	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Return the same layer data that matches the digest in the manifest
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(layerData)),
			}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, sourceECRClient, factory, httpClient, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:           "prod",
		Repo:          "myapp",
		SK:            "abc123",
		S3Bucket:      "bucket",
		S3Key:         "myapp/main/1.0.0",
		TargetAccount: "123456789012",
		TargetRegion:  "eu-west-1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
	// The manifest has 3 references (config + 2 layers) using the same digest.
	// Each is processed separately (future optimization: deduplicate digests).
	if layerUploadCount != 3 {
		t.Errorf("expected 3 layers uploaded, got %d", layerUploadCount)
	}
}

func TestHandlePromoteImages_CrossAccount_LayersAlreadyExist(t *testing.T) {
	// Test cross-account promotion when layers already exist in target
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	dockerManifest := `{
		"schemaVersion": 2,
		"config": {"digest": "sha256:config123"},
		"layers": [{"digest": "sha256:layer1"}]
	}`

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	sourceECRClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(dockerManifest)},
				},
			}, nil
		},
	}

	targetECRClient := &mockECRClient{
		batchCheckLayerAvailabilityFunc: func(ctx context.Context, params *ecr.BatchCheckLayerAvailabilityInput, optFns ...func(*ecr.Options)) (*ecr.BatchCheckLayerAvailabilityOutput, error) {
			// All layers already exist
			var layers []ecrtypes.Layer
			for _, digest := range params.LayerDigests {
				layers = append(layers, ecrtypes.Layer{
					LayerDigest:       aws.String(digest),
					LayerAvailability: ecrtypes.LayerAvailabilityAvailable,
				})
			}
			return &ecr.BatchCheckLayerAvailabilityOutput{Layers: layers}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	factory := &mockECRClientFactory{
		createClientFunc: func(ctx context.Context, targetAccount, targetRegion string) (ECRClient, error) {
			return targetECRClient, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, sourceECRClient, factory, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:           "prod",
		Repo:          "myapp",
		SK:            "abc123",
		S3Bucket:      "bucket",
		S3Key:         "myapp/main/1.0.0",
		TargetAccount: "123456789012",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_RepositoryDoesNotExist_CreatesIt(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	repoCreated := false
	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		describeRepositoriesFunc: func(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
			// Repository doesn't exist
			return nil, &ecrtypes.RepositoryNotFoundException{Message: aws.String("repository not found")}
		},
		createRepositoryFunc: func(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
			repoCreated = true
			// Verify settings
			if params.ImageTagMutability != ecrtypes.ImageTagMutabilityImmutable {
				t.Error("expected ImageTagMutability to be IMMUTABLE")
			}
			if params.ImageScanningConfiguration == nil || !params.ImageScanningConfiguration.ScanOnPush {
				t.Error("expected ScanOnPush to be true")
			}
			return &ecr.CreateRepositoryOutput{
				Repository: &ecrtypes.Repository{
					RepositoryName: params.RepositoryName,
				},
			}, nil
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repoCreated {
		t.Error("expected repository to be created")
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_RepositoryCreationRaceCondition(t *testing.T) {
	// Test that we handle RepositoryAlreadyExistsException gracefully
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		describeRepositoriesFunc: func(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
			// Repository doesn't exist
			return nil, &ecrtypes.RepositoryNotFoundException{Message: aws.String("repository not found")}
		},
		createRepositoryFunc: func(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
			// Race condition - another process created the repo
			return nil, &ecrtypes.RepositoryAlreadyExistsException{Message: aws.String("repository already exists")}
		},
		putImageFunc: func(ctx context.Context, params *ecr.PutImageInput, optFns ...func(*ecr.Options)) (*ecr.PutImageOutput, error) {
			return &ecr.PutImageOutput{}, nil
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	output, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	// Should succeed even with race condition
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ImagesPromoted != 1 {
		t.Errorf("expected ImagesPromoted=1, got %d", output.ImagesPromoted)
	}
}

func TestHandlePromoteImages_RepositoryCreateError(t *testing.T) {
	containerImages := ContainerImages{
		Images: []ContainerImage{
			{Name: "api", Registry: "myapp/api", Tag: "1.0.0"},
		},
	}

	s3Client := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return s3ContainerImagesResponse(containerImages), nil
		},
	}

	ecrClient := &mockECRClient{
		batchGetImageFunc: func(ctx context.Context, params *ecr.BatchGetImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchGetImageOutput, error) {
			return &ecr.BatchGetImageOutput{
				Images: []ecrtypes.Image{
					{ImageManifest: aws.String(`{"config":{}}`)},
				},
			}, nil
		},
		describeRepositoriesFunc: func(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
			return nil, &ecrtypes.RepositoryNotFoundException{Message: aws.String("repository not found")}
		},
		createRepositoryFunc: func(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
			return nil, errors.New("access denied")
		},
	}

	handler := NewHandlerWithDeps(s3Client, ecrClient, nil, nil, "us-east-1")

	_, err := handler.HandlePromoteImages(testContext(), &Input{
		Env:      "dev",
		Repo:     "myapp",
		SK:       "abc123",
		S3Bucket: "bucket",
		S3Key:    "myapp/main/1.0.0",
	})

	if err == nil {
		t.Fatal("expected error for repository creation failure")
	}
	if !strings.Contains(err.Error(), "failed to create repository") {
		t.Errorf("expected 'failed to create repository' error, got: %v", err)
	}
}
