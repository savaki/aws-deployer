package targetdao

func TableName(env string) string {
	return env + "-aws-deployer--targets"
}
