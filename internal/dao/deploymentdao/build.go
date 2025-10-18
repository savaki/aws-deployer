package deploymentdao

func TableName(env string) string {
	return env + "-aws-deployer--deployments"
}
