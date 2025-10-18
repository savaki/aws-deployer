package builddao

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/ddb/v2"
	"github.com/savaki/gox/slicex"
)

const latest = "latest"

// PK represents a DynamoDB partition key in format {repo}/{env}
// Example: myrepo/dev
type PK string

// NewPK creates a new partition key from repo and env
func NewPK(repo, env string) PK {
	return PK(fmt.Sprintf("%s/%s", repo, env))
}

// ParsePK parses a partition key into its repo and env components
func ParsePK(pk PK) (repo, env string, err error) {
	s := string(pk)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid PK format: %s, expected {repo}/{env}", s)
	}
	return parts[0], parts[1], nil
}

// String returns the string representation of the partition key
func (pk PK) String() string {
	return string(pk)
}

// ID represents a build ID in format {repo}/{env}:{ksuid}
// Example: myrepo/dev:2HFj3kLmNoPqRsTuVwXy
type ID string

func (id ID) String() string {
	return string(id)
}

// ParseID parses a build ID into its partition key (pk) and sort key (sk) components
func ParseID(id ID) (pk PK, sk string, err error) {
	s := string(id)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid build ID format: %s, expected {repo}/{env}:{ksuid}", s)
	}
	return PK(parts[0]), parts[1], nil
}

// NewID constructs an ID from partition key and sort key
func NewID(pk PK, sk string) ID {
	return ID(fmt.Sprintf("%s:%s", pk, sk))
}

// BuildStatus represents the current status of a deployment build
type BuildStatus string

const (
	BuildStatusPending    BuildStatus = "PENDING"
	BuildStatusInProgress BuildStatus = "IN_PROGRESS"
	BuildStatusSuccess    BuildStatus = "SUCCESS"
	BuildStatusFailed     BuildStatus = "FAILED"
)

// Record represents a deployment build record in DynamoDB
type Record struct {
	PK           PK          `ddb:"hash" dynamodbav:"pk"`          // {repo}/{env} - DynamoDB partition key
	SK           string      `ddb:"range" dynamodbav:"sk"`         // KSUID - DynamoDB sort key
	ID           ID          `dynamodbav:"id,omitempty"`           // ID is only used for latest entries
	Repo         string      `dynamodbav:"repo,omitempty"`         // Repository name only
	Env          string      `dynamodbav:"env,omitempty"`          // Environment name (dev, staging, prod)
	BuildNumber  string      `dynamodbav:"build_number,omitempty"` // Build number from version
	Branch       string      `dynamodbav:"branch,omitempty"`
	Version      string      `dynamodbav:"version,omitempty"`
	CommitHash   string      `dynamodbav:"commit_hash,omitempty"`
	Status       BuildStatus `dynamodbav:"status,omitempty"`
	StackName    string      `dynamodbav:"stack_name,omitempty"`
	ExecutionArn *string     `dynamodbav:"execution_arn,omitempty,omitempty"` // Step Functions execution ARN
	ErrorMsg     *string     `dynamodbav:"error_msg,omitempty,omitempty"`
	CreatedAt    int64       `dynamodbav:"created_at,omitempty"`            // Unix epoch timestamp of creation
	FinishedAt   *int64      `dynamodbav:"finished_at,omitempty,omitempty"` // Unix epoch timestamp of completion
	UpdatedAt    int64       `dynamodbav:"updated_at,omitempty"`            // Unix epoch timestamp of last update
}

// GetID returns the full build ID in format: {repo}/{env}:{ksuid}
func (r *Record) GetID() ID {
	if r.ID != "" {
		return r.ID
	}
	return NewID(r.PK, r.SK)
}

// CreateInput contains the fields needed to create a new build record
type CreateInput struct {
	Repo        string // Repository name
	Env         string // Environment (dev, staging, prod)
	SK          string // KSUID sort key
	BuildNumber string // Build number from version
	Branch      string // Git branch
	Version     string // Version string
	CommitHash  string // Git commit hash
	StackName   string // CloudFormation stack name
}

// UpdateInput contains the fields that can be updated on a build record
type UpdateInput struct {
	PK       PK           // Partition key (repo/env)
	SK       string       // Sort key (KSUID)
	Status   *BuildStatus // New status
	ErrorMsg *string      // Error message (optional)
}

// DAO provides data access operations for build records
type DAO struct {
	db    *ddb.DDB
	table *ddb.Table
}

// New creates a new DAO instance
func New(client *dynamodb.Client, tableName string) *DAO {
	db := ddb.New(client)
	table := db.MustTable(tableName, &Record{})
	return &DAO{
		db:    db,
		table: table,
	}
}

// Create creates a new build record with initial status PENDING
func (d *DAO) Create(ctx context.Context, input CreateInput) (Record, error) {
	pk := NewPK(input.Repo, input.Env)
	now := time.Now().Unix()

	record := Record{
		PK:          pk,
		SK:          input.SK,
		Repo:        input.Repo,
		Env:         input.Env,
		BuildNumber: input.BuildNumber,
		Branch:      input.Branch,
		Version:     input.Version,
		CommitHash:  input.CommitHash,
		Status:      BuildStatusPending,
		StackName:   input.StackName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := d.table.Put(&record).RunWithContext(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("failed to create build record: %w", err)
	}

	return record, nil
}

// Find retrieves a build record by ID
// Returns an error if not found or if there's a database error
func (d *DAO) Find(ctx context.Context, id ID) (Record, error) {
	pk, sk, err := ParseID(id)
	if err != nil {
		return Record{}, err
	}

	var record Record

	err = d.table.Get(pk.String()).
		Range(sk).
		ConsistentRead(true).
		ScanWithContext(ctx, &record)
	if err != nil {
		// Check if it's a "not found" error
		errStr := err.Error()
		if strings.Contains(errStr, "item not found") || strings.Contains(errStr, "ItemNotFound") {
			return Record{}, fmt.Errorf("build record not found: %s", id)
		}
		return Record{}, fmt.Errorf("failed to find build record: %w", err)
	}

	// If all fields are empty, item doesn't exist
	if record.PK == "" && record.SK == "" {
		return Record{}, fmt.Errorf("build record not found: %s", id)
	}

	return record, nil
}

// Delete removes a build record by ID
func (d *DAO) Delete(ctx context.Context, id ID) error {
	pk, sk, err := ParseID(id)
	if err != nil {
		return err
	}

	err = d.table.Delete(pk).
		Range(sk).
		RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete build record: %w", err)
	}

	return nil
}

// UpdateStatus updates the status of a build record and creates/updates a "latest" magic record
// The latest record has pk=latest/{env} and sk={original pk} to enable efficient queries for latest builds
func (d *DAO) UpdateStatus(ctx context.Context, input UpdateInput) error {
	if input.Status == nil {
		return fmt.Errorf("status is required")
	}

	now := time.Now().Unix()

	// Build the update operation with chained Set calls
	update := d.table.Update(input.PK).
		Range(input.SK).
		Set("#Status = ?", string(*input.Status)).
		Set("#UpdatedAt = ?", now)

	// Set finishedAt for terminal states (SUCCESS or FAILED)
	if *input.Status == BuildStatusSuccess || *input.Status == BuildStatusFailed {
		update = update.Set("#FinishedAt = ?", now)
	}

	// Add error message if provided
	if input.ErrorMsg != nil {
		update = update.Set("#ErrorMsg = ?", *input.ErrorMsg)
	}

	// Create/update the "latest" magic record
	// Parse env from PK (format: {repo}/{env})
	repo, env, err := ParsePK(input.PK)
	if err != nil {
		return fmt.Errorf("failed to parse PK: %w", err)
	}

	latestRecord := &Record{
		PK:        NewPK(latest, env),
		SK:        input.PK.String(), // SK in latest record = PK from original (repo/env identifier)
		ID:        NewID(input.PK, input.SK),
		Repo:      repo,
		Env:       env,
		Status:    *input.Status,
		UpdatedAt: now,
	}

	// Write both the update and the latest record in a transaction
	put := d.table.Put(latestRecord)

	if _, err := d.db.TransactWriteItemsWithContext(ctx, update, put); err != nil {
		return err
	}

	return nil
}

// Query returns all builds for a given repo/env partition key
func (d *DAO) Query(ctx context.Context, pk PK) ([]Record, error) {
	var records []Record

	err := d.table.Query("#PK = ?", pk.String()).
		FindAllWithContext(ctx, &records)
	if err != nil {
		return nil, fmt.Errorf("failed to query builds: %w", err)
	}

	return records, nil
}

// QueryByRepoEnv returns all builds for a given repository and environment
func (d *DAO) QueryByRepoEnv(ctx context.Context, repo, env string) ([]Record, error) {
	pk := NewPK(repo, env)
	return d.Query(ctx, pk)
}

// QueryLatestBuilds returns the latest build for each repo in the given environment
// It queries the "latest" magic records where pk=latest/{env} and sk={repo}/{env}
func (d *DAO) QueryLatestBuilds(ctx context.Context, env string) ([]Record, error) {
	pk := NewPK(latest, env)
	var records []Record

	err := d.table.Query("#PK = ?", pk).
		FindAllWithContext(ctx, &records)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest builds: %w", err)
	}

	// Sort by UpdatedAt descending (most recent first)
	// The records are already sorted by SK (repo/env), but we want to sort by time
	for i := 0; i < len(records)-1; i++ {
		for j := i + 1; j < len(records); j++ {
			if records[j].UpdatedAt > records[i].UpdatedAt {
				records[i], records[j] = records[j], records[i]
			}
		}
	}

	ids := slicex.Map(records, GetID)

	// Load full build records for each ID
	builds := make([]Record, 0, len(ids))
	for _, id := range ids {
		record, err := d.Find(ctx, id)
		if err != nil {
			// Skip records that are not found (may have been deleted)
			continue
		}
		builds = append(builds, record)
	}

	return builds, nil
}

// StartExecution atomically updates a build record to IN_PROGRESS status and sets the execution ARN
// This should be called when a Step Functions execution is started for the build
// It also updates the "latest" magic record to ensure the latest build is reflected immediately
func (d *DAO) StartExecution(ctx context.Context, pk PK, sk string, executionArn string) error {
	now := time.Now().Unix()
	status := BuildStatusInProgress

	update := d.table.Update(pk.String()).
		Range(sk).
		Set("#Status = ?", string(status)).
		Set("#ExecutionArn = ?", executionArn).
		Set("#UpdatedAt = ?", now)

	// Create/update the "latest" magic record
	// Parse env from PK (format: {repo}/{env})
	repo, env, err := ParsePK(pk)
	if err != nil {
		return fmt.Errorf("failed to parse PK: %w", err)
	}

	latestRecord := &Record{
		PK:        NewPK(latest, env),
		SK:        pk.String(), // SK in latest record = PK from original (repo/env identifier)
		ID:        NewID(pk, sk),
		Repo:      repo,
		Env:       env,
		Status:    status,
		UpdatedAt: now,
	}

	// Write both the update and the latest record in a transaction
	put := d.table.Put(latestRecord)

	if _, err := d.db.TransactWriteItemsWithContext(ctx, update, put); err != nil {
		return fmt.Errorf("failed to start execution: %w", err)
	}

	return nil
}
