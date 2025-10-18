package lockdao

func TableName(env string) string {
	return env + "-aws-deployer--locks"
}
