package utils

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"maps"
	"slices"
)

// MergeParameters merges multiple parameter maps with later maps having higher precedence
// Returns a CloudFormation parameter list with merged results
func MergeParameters(pp ...map[string]string) []types.Parameter {
	m := map[string]string{}
	for _, p := range pp {
		maps.Copy(m, p)
	}

	var results []types.Parameter
	for _, k := range slices.Sorted(maps.Keys(m)) {
		v := m[k]
		results = append(results, types.Parameter{
			ParameterKey:   aws.String(k),
			ParameterValue: aws.String(v),
		})
	}

	return results
}
