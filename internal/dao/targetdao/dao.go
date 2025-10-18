package targetdao

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/ddb/v2"
)

const (
	// DefaultRepo is the special repo identifier for default configuration
	DefaultRepo = "$"
	// ConfigEnv is the special SK identifier for configuration records
	ConfigEnv = "$"
)

// PK represents the partition key (repo name, use DefaultRepo for default)
type PK string

// NewPK creates a partition key from a repo name
func NewPK(repo string) PK {
	return PK(repo)
}

// String returns the string representation
func (pk PK) String() string {
	return string(pk)
}

// ID represents a target configuration ID in format {repo}:{env}
// Example: my-repo:dev or $:prod (for default)
type ID string

// NewID creates an ID from repo and env
func NewID(repo, env string) ID {
	return ID(fmt.Sprintf("%s:%s", repo, env))
}

// ParseID parses an ID into repo and env components
func ParseID(id ID) (repo, env string, err error) {
	s := string(id)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid ID format: %s, expected {repo}:{env}", s)
	}
	return parts[0], parts[1], nil
}

// String returns the string representation
func (id ID) String() string {
	return string(id)
}

// Target represents a deployment target with account IDs and regions
type Target struct {
	AccountIDs []string `json:"account_ids" dynamodbav:"account_ids"`
	Regions    []string `json:"regions" dynamodbav:"regions"`
}

// Record represents a deployment target configuration
type Record struct {
	PK            PK       `ddb:"hash" dynamodbav:"pk"`                  // repo name (use DefaultRepo for default)
	SK            string   `ddb:"range" dynamodbav:"sk"`                 // environment (or ConfigEnv for config)
	Targets       []Target `dynamodbav:"targets,omitempty"`              // list of account/region targets (when SK is env)
	InitialEnv    string   `dynamodbav:"initial_env,omitempty"`          // initial environment (when SK is ConfigEnv)
	DownstreamEnv []string `dynamodbav:"downstream_env,omitempty"`       // downstream environments (when SK is env)
}

// GetID returns the ID for this record
func (r *Record) GetID() ID {
	return NewID(r.PK.String(), r.SK)
}

// CreateInput contains fields for creating a targets configuration
type CreateInput struct {
	Repo          string   // Repository name (use DefaultRepo for default)
	Env           string   // Environment (or ConfigEnv for config)
	Targets       []Target // List of account/region targets (when Env is env)
	InitialEnv    string   // Initial environment (when Env is ConfigEnv)
	DownstreamEnv []string // Downstream environments (when Env is env)
}

// UpdateInput contains fields for updating a targets configuration
type UpdateInput struct {
	ID            ID       // Target configuration ID
	Targets       []Target // New list of account/region targets
	InitialEnv    string   // Initial environment (when updating config)
	DownstreamEnv []string // Downstream environments (when updating env targets)
}

// DAO provides data access operations for deployment targets
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

// Find retrieves a targets configuration by ID
// Returns nil if not found
func (d *DAO) Find(ctx context.Context, id ID) (*Record, error) {
	repo, env, err := ParseID(id)
	if err != nil {
		return nil, err
	}

	pk := NewPK(repo)
	var record Record

	err = d.table.Get(pk.String()).
		Range(env).
		ConsistentRead(true).
		ScanWithContext(ctx, &record)
	if err != nil {
		errStr := err.Error()
		// Check if item not found
		if contains(errStr, "item not found") || contains(errStr, "ItemNotFound") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get targets: %w", err)
	}

	// If all fields are empty, item doesn't exist
	if record.PK == "" && record.SK == "" {
		return nil, nil
	}

	return &record, nil
}

// Get retrieves the targets for a given repo and environment
// Returns nil if no targets are configured
// Deprecated: Use Find(ctx, NewID(repo, env)) instead
func (d *DAO) Get(ctx context.Context, repo, env string) (*Record, error) {
	return d.Find(ctx, NewID(repo, env))
}

// GetWithDefault retrieves targets for a repo/env, falling back to default (DefaultRepo) if not found
func (d *DAO) GetWithDefault(ctx context.Context, repo, env string) (*Record, error) {
	// Try repo-specific targets first
	record, err := d.Find(ctx, NewID(repo, env))
	if err != nil {
		return nil, err
	}
	if record != nil {
		return record, nil
	}

	// Fall back to default targets
	record, err = d.Find(ctx, NewID(DefaultRepo, env))
	if err != nil {
		return nil, fmt.Errorf("failed to get default targets: %w", err)
	}

	return record, nil
}

// Create creates a new targets configuration
func (d *DAO) Create(ctx context.Context, input CreateInput) (*Record, error) {
	record := &Record{
		PK:            NewPK(input.Repo),
		SK:            input.Env,
		Targets:       input.Targets,
		InitialEnv:    input.InitialEnv,
		DownstreamEnv: input.DownstreamEnv,
	}

	err := d.table.Put(record).RunWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create targets: %w", err)
	}

	return record, nil
}

// Update updates a targets configuration
func (d *DAO) Update(ctx context.Context, input UpdateInput) (*Record, error) {
	repo, env, err := ParseID(input.ID)
	if err != nil {
		return nil, err
	}

	record := &Record{
		PK:            NewPK(repo),
		SK:            env,
		Targets:       input.Targets,
		InitialEnv:    input.InitialEnv,
		DownstreamEnv: input.DownstreamEnv,
	}

	err = d.table.Put(record).RunWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update targets: %w", err)
	}

	return record, nil
}

// Put creates or updates a targets configuration
// Deprecated: Use Create or Update instead
func (d *DAO) Put(ctx context.Context, repo, env string, targets []Target) error {
	_, err := d.Create(ctx, CreateInput{
		Repo:    repo,
		Env:     env,
		Targets: targets,
	})
	return err
}

// Delete removes a targets configuration
func (d *DAO) Delete(ctx context.Context, id ID) error {
	repo, env, err := ParseID(id)
	if err != nil {
		return err
	}

	pk := NewPK(repo)

	err = d.table.Delete(pk.String()).
		Range(env).
		RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete targets: %w", err)
	}

	return nil
}

// GetConfig retrieves the configuration (initial env) for a repo or default
// Returns nil if no configuration is set
func (d *DAO) GetConfig(ctx context.Context, repo string) (*Record, error) {
	return d.Find(ctx, NewID(repo, ConfigEnv))
}

// SetConfig sets the configuration (initial env) for a repo or default
func (d *DAO) SetConfig(ctx context.Context, repo, initialEnv string) (*Record, error) {
	return d.Create(ctx, CreateInput{
		Repo:       repo,
		Env:        ConfigEnv,
		InitialEnv: initialEnv,
	})
}

// GetInitialEnv gets the initial environment for a repo, falling back to default
// Returns "dev" as the ultimate fallback if nothing is configured
func (d *DAO) GetInitialEnv(ctx context.Context, repo string) (string, error) {
	// Try repo-specific config first
	config, err := d.GetConfig(ctx, repo)
	if err != nil {
		return "", err
	}
	if config != nil && config.InitialEnv != "" {
		return config.InitialEnv, nil
	}

	// Fall back to default config
	config, err = d.GetConfig(ctx, DefaultRepo)
	if err != nil {
		return "", err
	}
	if config != nil && config.InitialEnv != "" {
		return config.InitialEnv, nil
	}

	// Ultimate fallback
	return "dev", nil
}

// FindAll scans all records in the targets table
func (d *DAO) FindAll(ctx context.Context) ([]*Record, error) {
	var records []*Record
	err := d.table.Scan().ConsistentRead(false).EachWithContext(ctx, func(item ddb.Item) (bool, error) {
		var record Record
		if err := item.Unmarshal(&record); err != nil {
			return false, err
		}
		records = append(records, &record)
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan targets: %w", err)
	}
	return records, nil
}

// ExpandTargets converts targets into a list of all account/region combinations
func ExpandTargets(targets []Target) []struct{ AccountID, Region string } {
	var result []struct{ AccountID, Region string }

	for _, target := range targets {
		for _, accountID := range target.AccountIDs {
			for _, region := range target.Regions {
				result = append(result, struct{ AccountID, Region string }{
					AccountID: accountID,
					Region:    region,
				})
			}
		}
	}

	return result
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
