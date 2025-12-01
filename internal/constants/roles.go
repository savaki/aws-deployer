package constants

// StackSet role names used for multi-account deployments
const (
	// ExecutionRoleName is the name of the role in target accounts that
	// CloudFormation StackSets assume to perform deployments
	ExecutionRoleName = "AWSCloudFormationStackSetExecutionRole"

	// AdministrationRoleName is the name of the role in the deployer account
	// that CloudFormation uses to orchestrate StackSet operations
	AdministrationRoleName = "AWSCloudFormationStackSetAdministrationRole"

	// ECRImagePromotionRoleName is the name of the role in target accounts
	// that allows ECR image promotion (create repos, push images only)
	ECRImagePromotionRoleName = "ECRImagePromotionRole"
)
