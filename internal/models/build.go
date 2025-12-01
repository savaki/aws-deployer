package models

type StepFunctionInput struct {
	Repo       string `json:"repo"`        // Repository name
	Env        string `json:"env"`         // Environment name (dev, staging, prod)
	Branch     string `json:"branch"`      // Git branch
	Version    string `json:"version"`     // Version string
	SK         string `json:"sk"`          // KSUID - DynamoDB sort key
	CommitHash string `json:"commit_hash"` // Git commit hash
	S3Bucket   string `json:"s3_bucket"`   // S3 bucket for artifacts
	S3Key      string `json:"s3_key"`      // S3 key prefix for artifacts
}
