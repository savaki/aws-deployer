package deploymentdao

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/ddb/v2"
)

// PK represents the partition key: {Env}/{Repository}
type PK string

// NewPK creates a partition key from env and repo
func NewPK(env, repo string) PK {
	return PK(fmt.Sprintf("%s/%s", env, repo))
}

// ParsePK parses a partition key into env and repo components
func ParsePK(pk PK) (env, repo string, err error) {
	s := string(pk)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid PK format: %s, expected {env}/{repo}", s)
	}
	return parts[0], parts[1], nil
}

// String returns the string representation
func (pk PK) String() string {
	return string(pk)
}

// SK represents the sort key: {Account}/{Region}
type SK string

// NewSK creates a sort key from account and region
func NewSK(account, region string) SK {
	return SK(fmt.Sprintf("%s/%s", account, region))
}

// String returns the string representation
func (sk SK) String() string {
	return string(sk)
}

// ParseSK parses a sort key into account and region components
func ParseSK(sk SK) (account, region string, err error) {
	s := string(sk)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid SK format: %s, expected {account}/{region}", s)
	}
	return parts[0], parts[1], nil
}

// ID represents a deployment ID in format {env}/{repo}:{account}/{region}
// Example: dev/my-repo:111111111111/us-east-1
type ID string

// NewID creates an ID from env, repo, account, and region
func NewID(env, repo, account, region string) ID {
	pk := NewPK(env, repo)
	sk := NewSK(account, region)
	return ID(fmt.Sprintf("%s:%s", pk, sk))
}

// ParseID parses an ID into env, repo, account, and region components
func ParseID(id ID) (env, repo, account, region string, err error) {
	s := string(id)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", "", "", "", fmt.Errorf("invalid ID format: %s, expected {env}/{repo}:{account}/{region}", s)
	}

	// Parse PK part
	pkParts := strings.Split(parts[0], "/")
	if len(pkParts) != 2 {
		return "", "", "", "", fmt.Errorf("invalid PK in ID: %s, expected {env}/{repo}", parts[0])
	}

	// Parse SK part
	skParts := strings.Split(parts[1], "/")
	if len(skParts) != 2 {
		return "", "", "", "", fmt.Errorf("invalid SK in ID: %s, expected {account}/{region}", parts[1])
	}

	return pkParts[0], pkParts[1], skParts[0], skParts[1], nil
}

// String returns the string representation
func (id ID) String() string {
	return string(id)
}

// DeploymentStatus represents the status of a deployment
type DeploymentStatus string

const (
	StatusPending    DeploymentStatus = "PENDING"
	StatusInProgress DeploymentStatus = "IN_PROGRESS"
	StatusSuccess    DeploymentStatus = "SUCCESS"
	StatusFailed     DeploymentStatus = "FAILED"
)

// Record represents a single account/region deployment state
type Record struct {
	PK           PK               `ddb:"hash" dynamodbav:"pk"`           // {Env}/{Repository}
	SK           SK               `ddb:"range" dynamodbav:"sk"`          // {Account}/{Region}
	BuildID      string           `dynamodbav:"build_id"`                // KSUID linking to build record
	StackID      string           `dynamodbav:"stack_id,omitempty"`      // CloudFormation stack ID
	Status       DeploymentStatus `dynamodbav:"status"`                  // PENDING|IN_PROGRESS|SUCCESS|FAILED
	OperationID  string           `dynamodbav:"operation_id,omitempty"`  // StackSet operation ID
	StatusReason string           `dynamodbav:"status_reason,omitempty"` // CF status reason
	ErrorMsg     string           `dynamodbav:"error_msg,omitempty"`     // Detailed failure message
	StackEvents  []string         `dynamodbav:"stack_events,omitempty"`  // Recent failed events
	CreatedAt    int64            `dynamodbav:"created_at"`              // Unix timestamp
	UpdatedAt    int64            `dynamodbav:"updated_at"`              // Unix timestamp
	FinishedAt   int64            `dynamodbav:"finished_at,omitempty"`   // Unix timestamp
}

// GetID returns the ID for this record
func (r *Record) GetID() ID {
	env, repo, _ := ParsePK(r.PK)
	account, region, _ := ParseSK(r.SK)
	return NewID(env, repo, account, region)
}

// CreateInput contains fields for creating a deployment record
type CreateInput struct {
	Env     string // Environment
	Repo    string // Repository
	Account string // AWS Account ID
	Region  string // AWS Region
	BuildID string // Build KSUID
}

// DAO provides data access operations for deployment tracking
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

// Create initializes a deployment record with PENDING status
func (d *DAO) Create(ctx context.Context, input CreateInput) (Record, error) {
	now := time.Now().Unix()

	record := Record{
		PK:        NewPK(input.Env, input.Repo),
		SK:        NewSK(input.Account, input.Region),
		BuildID:   input.BuildID,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := d.table.Put(&record).RunWithContext(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("failed to create deployment record: %w", err)
	}

	return record, nil
}

// Find retrieves a deployment record by ID
// Returns an error if not found or if there's a database error
func (d *DAO) Find(ctx context.Context, id ID) (Record, error) {
	env, repo, account, region, err := ParseID(id)
	if err != nil {
		return Record{}, err
	}

	pk := NewPK(env, repo)
	sk := NewSK(account, region)
	var record Record

	err = d.table.Get(pk.String()).
		Range(sk.String()).
		ConsistentRead(true).
		ScanWithContext(ctx, &record)
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "item not found") || contains(errStr, "ItemNotFound") {
			return Record{}, fmt.Errorf("deployment record not found: %s", id)
		}
		return Record{}, fmt.Errorf("failed to get deployment: %w", err)
	}

	if record.PK == "" && record.SK == "" {
		return Record{}, fmt.Errorf("deployment record not found: %s", id)
	}

	return record, nil
}

// Get retrieves a deployment record
// Deprecated: Use Find(ctx, NewID(env, repo, account, region)) instead
func (d *DAO) Get(ctx context.Context, env, repo, account, region string) (Record, error) {
	return d.Find(ctx, NewID(env, repo, account, region))
}

// UpdateInput contains fields for updating a deployment record
type UpdateInput struct {
	Env          string
	Repo         string
	Account      string
	Region       string
	Status       DeploymentStatus
	StackID      string
	OperationID  string
	StatusReason string
	ErrorMsg     string
	StackEvents  []string
}

// UpdateStatus updates a deployment record with new status and failure information
func (d *DAO) UpdateStatus(ctx context.Context, input UpdateInput) error {
	pk := NewPK(input.Env, input.Repo)
	sk := NewSK(input.Account, input.Region)
	now := time.Now().Unix()

	update := d.table.Update(pk.String()).
		Range(sk.String()).
		Set("#Status = ?", string(input.Status)).
		Set("#UpdatedAt = ?", now)

	if input.StackID != "" {
		update = update.Set("#StackID = ?", input.StackID)
	}

	if input.OperationID != "" {
		update = update.Set("#OperationID = ?", input.OperationID)
	}

	if input.StatusReason != "" {
		update = update.Set("#StatusReason = ?", input.StatusReason)
	}

	if input.ErrorMsg != "" {
		update = update.Set("#ErrorMsg = ?", input.ErrorMsg)
	}

	if len(input.StackEvents) > 0 {
		update = update.Set("#StackEvents = ?", input.StackEvents)
	}

	// Set finished_at for terminal states
	if input.Status == StatusSuccess || input.Status == StatusFailed {
		update = update.Set("#FinishedAt = ?", now)
	}

	err := update.RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	return nil
}

// QueryByBuild returns all deployments for a given build
func (d *DAO) QueryByBuild(ctx context.Context, env, repo, buildID string) ([]Record, error) {
	pk := NewPK(env, repo)
	var records []Record

	err := d.table.Query("#PK = ?", pk).
		Filter("#BuildID = ?", buildID).
		FindAllWithContext(ctx, &records)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployments: %w", err)
	}

	return records, nil
}

// QueryByPK returns all deployments for a given env/repo partition key
func (d *DAO) QueryByPK(ctx context.Context, env, repo string) ([]Record, error) {
	pk := NewPK(env, repo)
	var records []Record

	err := d.table.Query("#PK = ?", pk).
		FindAllWithContext(ctx, &records)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployments: %w", err)
	}

	return records, nil
}

// Delete removes a deployment record
func (d *DAO) Delete(ctx context.Context, id ID) error {
	env, repo, account, region, err := ParseID(id)
	if err != nil {
		return err
	}

	pk := NewPK(env, repo)
	sk := NewSK(account, region)

	err = d.table.Delete(pk.String()).
		Range(sk.String()).
		RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
