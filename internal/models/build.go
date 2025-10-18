package models

type StepFunctionInput struct {
	Repo       string `json:"repo"` // Repository name only
	Env        string `json:"env"`  // Environment name (dev, staging, prod)
	Branch     string `json:"branch"`
	Version    string `json:"version"`
	SK         string `json:"sk"` // KSUID - DynamoDB sort key
	CommitHash string `json:"commit_hash"`
	S3Bucket   string `json:"s3_bucket"`
	S3Key      string `json:"s3_key"`
}
