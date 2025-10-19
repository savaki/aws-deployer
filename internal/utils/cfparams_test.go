package utils

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestMergeParameters(t *testing.T) {
	tests := []struct {
		name   string
		inputs []map[string]string
		want   []types.Parameter
	}{
		{
			name: "single map",
			inputs: []map[string]string{
				{"Env": "dev", "Version": "1.0.0"},
			},
			want: []types.Parameter{
				{ParameterKey: aws.String("Env"), ParameterValue: aws.String("dev")},
				{ParameterKey: aws.String("Version"), ParameterValue: aws.String("1.0.0")},
			},
		},
		{
			name: "merge two maps - override wins",
			inputs: []map[string]string{
				{"Env": "dev", "Version": "1.0.0", "S3Bucket": "my-bucket"},
				{"Env": "prod", "Region": "us-west-2"},
			},
			want: []types.Parameter{
				{ParameterKey: aws.String("Env"), ParameterValue: aws.String("prod")},
				{ParameterKey: aws.String("Region"), ParameterValue: aws.String("us-west-2")},
				{ParameterKey: aws.String("S3Bucket"), ParameterValue: aws.String("my-bucket")},
				{ParameterKey: aws.String("Version"), ParameterValue: aws.String("1.0.0")},
			},
		},
		{
			name:   "empty maps",
			inputs: []map[string]string{},
			want:   []types.Parameter{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeParameters(tt.inputs...)

			if len(got) != len(tt.want) {
				t.Errorf("MergeParameters() length = %v, want %v", len(got), len(tt.want))
				return
			}

			// Convert to maps for easier comparison (order doesn't matter)
			gotMap := make(map[string]string)
			for _, param := range got {
				gotMap[aws.ToString(param.ParameterKey)] = aws.ToString(param.ParameterValue)
			}

			wantMap := make(map[string]string)
			for _, param := range tt.want {
				wantMap[aws.ToString(param.ParameterKey)] = aws.ToString(param.ParameterValue)
			}

			for key, wantVal := range wantMap {
				gotVal, ok := gotMap[key]
				if !ok {
					t.Errorf("MergeParameters() missing key %v", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("MergeParameters() key %v = %v, want %v", key, gotVal, wantVal)
				}
			}

			for key := range gotMap {
				if _, ok := wantMap[key]; !ok {
					t.Errorf("MergeParameters() unexpected key %v", key)
				}
			}
		})
	}
}
