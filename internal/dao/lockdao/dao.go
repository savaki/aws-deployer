package lockdao

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/ddb/v2"
)

const (
	lockSK         = "LOCK"
	lockTTLHours   = 4 // Auto-expire locks after 4 hours
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

// ID represents a lock ID in format {env}/{repo}:LOCK
// Example: dev/my-repo:LOCK
// Note: SK is always "LOCK" so ID primarily identifies the env/repo
type ID string

// NewID creates an ID from env and repo
func NewID(env, repo string) ID {
	pk := NewPK(env, repo)
	return ID(fmt.Sprintf("%s:%s", pk, lockSK))
}

// ParseID parses an ID into env and repo components
func ParseID(id ID) (env, repo string, err error) {
	s := string(id)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid ID format: %s, expected {env}/{repo}:LOCK", s)
	}

	// Verify SK is LOCK
	if parts[1] != lockSK {
		return "", "", fmt.Errorf("invalid ID format: %s, expected SK to be 'LOCK', got '%s'", s, parts[1])
	}

	// Parse PK part
	pkParts := strings.Split(parts[0], "/")
	if len(pkParts) != 2 {
		return "", "", fmt.Errorf("invalid PK in ID: %s, expected {env}/{repo}", parts[0])
	}

	return pkParts[0], pkParts[1], nil
}

// String returns the string representation
func (id ID) String() string {
	return string(id)
}

// Record represents a deployment lock
type Record struct {
	PK           PK     `ddb:"hash" dynamodbav:"pk"`  // {Env}/{Repository}
	SK           string `ddb:"range" dynamodbav:"sk"` // Always "LOCK"
	BuildID      string `dynamodbav:"build_id"`       // KSUID of the build holding the lock
	ExecutionArn string `dynamodbav:"execution_arn"`  // Step Function execution ARN
	AcquiredAt   int64  `dynamodbav:"acquired_at"`    // Unix timestamp when lock was acquired
	TTL          int64  `dynamodbav:"ttl"`            // Unix timestamp for DynamoDB TTL expiry
}

// GetID returns the ID for this record
func (r *Record) GetID() ID {
	env, repo, _ := ParsePK(r.PK)
	return NewID(env, repo)
}

// AcquireInput contains fields for acquiring a deployment lock
type AcquireInput struct {
	Env          string // Environment
	Repo         string // Repository
	BuildID      string // Build KSUID
	ExecutionArn string // Step Function execution ARN
}

// ReleaseInput contains fields for releasing a deployment lock
type ReleaseInput struct {
	ID      ID     // Lock ID
	BuildID string // Build KSUID (must match lock holder)
}

// DAO provides data access operations for deployment locks
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

// Acquire attempts to acquire a deployment lock
// Returns the lock record if acquired, nil if already held by another build
func (d *DAO) Acquire(ctx context.Context, input AcquireInput) (*Record, bool, error) {
	id := NewID(input.Env, input.Repo)

	// Check if lock already exists
	existing, err := d.Find(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check existing lock: %w", err)
	}

	if existing != nil {
		// Lock is held by another build (or same build on retry)
		if existing.BuildID == input.BuildID {
			// Same build already holds the lock (retry scenario)
			return existing, true, nil
		}
		// Different build holds the lock
		return nil, false, nil
	}

	// No lock exists, create it
	now := time.Now().Unix()
	ttl := now + (lockTTLHours * 3600)

	pk := NewPK(input.Env, input.Repo)
	record := &Record{
		PK:           pk,
		SK:           lockSK,
		BuildID:      input.BuildID,
		ExecutionArn: input.ExecutionArn,
		AcquiredAt:   now,
		TTL:          ttl,
	}

	err = d.table.Put(record).RunWithContext(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create lock: %w", err)
	}

	return record, true, nil
}

// TryAcquire attempts to acquire a deployment lock
// Returns true if lock was acquired, false if already held by another build
// Deprecated: Use Acquire instead
func (d *DAO) TryAcquire(ctx context.Context, env, repo, buildID, executionArn string) (bool, error) {
	_, acquired, err := d.Acquire(ctx, AcquireInput{
		Env:          env,
		Repo:         repo,
		BuildID:      buildID,
		ExecutionArn: executionArn,
	})
	return acquired, err
}

// Find retrieves a lock record by ID
// Returns nil if not found
func (d *DAO) Find(ctx context.Context, id ID) (*Record, error) {
	env, repo, err := ParseID(id)
	if err != nil {
		return nil, err
	}

	pk := NewPK(env, repo)
	var record Record

	err = d.table.Get(pk.String()).
		Range(lockSK).
		ConsistentRead(true).
		ScanWithContext(ctx, &record)
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "item not found") || contains(errStr, "ItemNotFound") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get lock: %w", err)
	}

	if record.PK == "" && record.SK == "" {
		return nil, nil
	}

	return &record, nil
}

// Get retrieves the current lock holder (if any)
// Deprecated: Use Find(ctx, NewID(env, repo)) instead
func (d *DAO) Get(ctx context.Context, env, repo string) (*Record, error) {
	return d.Find(ctx, NewID(env, repo))
}

// Release releases a deployment lock
// Only succeeds if the lock is held by the specified buildID (prevents unauthorized releases)
func (d *DAO) Release(ctx context.Context, input ReleaseInput) error {
	env, repo, err := ParseID(input.ID)
	if err != nil {
		return err
	}

	// Verify lock is held by this build before releasing
	existing, err := d.Find(ctx, input.ID)
	if err != nil {
		return fmt.Errorf("failed to check lock: %w", err)
	}

	if existing == nil {
		// No lock exists (already released or expired)
		return nil
	}

	if existing.BuildID != input.BuildID {
		return fmt.Errorf("lock not held by build %s (held by %s)", input.BuildID, existing.BuildID)
	}

	// Delete the lock
	pk := NewPK(env, repo)
	err = d.table.Delete(pk.String()).
		Range(lockSK).
		RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	return nil
}

// Delete removes a lock record
func (d *DAO) Delete(ctx context.Context, id ID) error {
	env, repo, err := ParseID(id)
	if err != nil {
		return err
	}

	pk := NewPK(env, repo)

	err = d.table.Delete(pk.String()).
		Range(lockSK).
		RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	return nil
}

// ForceRelease releases a lock regardless of who holds it
// Use with caution - only for cleanup/recovery scenarios
// Deprecated: Use Delete(ctx, NewID(env, repo)) instead
func (d *DAO) ForceRelease(ctx context.Context, env, repo string) error {
	return d.Delete(ctx, NewID(env, repo))
}

func stringPtr(s string) *string {
	return &s
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
