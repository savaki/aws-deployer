package di

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
)

func ProvideBuildDAO(env string, client *dynamodb.Client) *builddao.DAO {
	return builddao.New(client, builddao.TableName(env))
}

func ProvideTargetDAO(env string, client *dynamodb.Client) *targetdao.DAO {
	return targetdao.New(client, targetdao.TableName(env))
}

func ProvideDeploymentDAO(env string, client *dynamodb.Client) *deploymentdao.DAO {
	return deploymentdao.New(client, deploymentdao.TableName(env))
}
