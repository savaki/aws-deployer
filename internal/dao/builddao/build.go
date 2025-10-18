package builddao

func TableName(env string) string {
	return env + "-aws-deployer--builds"
}
