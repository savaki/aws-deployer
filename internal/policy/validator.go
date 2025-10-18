package policy

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
)

//go:embed cloudformation.rego
var policyContent string

type Validator struct {
	prepared rego.PreparedEvalQuery
}

type ValidationResult struct {
	Allowed    bool     `json:"allowed"`
	Violations []string `json:"violations,omitempty"`
}

func NewValidator() (*Validator, error) {
	query, err := rego.New(
		rego.Query("data.cloudformation.allow"),
		rego.Module("cloudformation.rego", policyContent),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to prepare policy query: %w", err)
	}

	return &Validator{
		prepared: query,
	}, nil
}

func (v *Validator) ValidateTemplate(template map[string]interface{}, env, repo string) (*ValidationResult, error) {
	ctx := context.Background()

	// Create input data for the policy evaluation
	input := map[string]interface{}{
		"Resources": template["Resources"],
	}

	// Create data context for the policy
	data := map[string]interface{}{
		"env":  env,
		"repo": repo,
	}

	// Create an in-memory store with the data
	store := inmem.NewFromObject(data)

	// Create a new rego query with data for this evaluation
	query, err := rego.New(
		rego.Query("data.cloudformation.allow"),
		rego.Module("cloudformation.rego", policyContent),
		rego.Store(store),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare policy query with data: %w", err)
	}

	// Evaluate the allow rule
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate policy: %w", err)
	}

	if len(results) == 0 {
		return &ValidationResult{
			Allowed:    false,
			Violations: []string{"policy evaluation returned no results"},
		}, nil
	}

	allowed, ok := results[0].Expressions[0].Value.(bool)
	if !ok {
		return &ValidationResult{
			Allowed:    false,
			Violations: []string{"policy evaluation returned non-boolean result"},
		}, nil
	}

	result := &ValidationResult{
		Allowed: allowed,
	}

	// If not allowed, get violations
	if !allowed {
		violations, err := v.getViolations(ctx, input, data)
		if err != nil {
			return nil, fmt.Errorf("failed to get violations: %w", err)
		}
		result.Violations = violations
	}

	return result, nil
}

func (v *Validator) getViolations(ctx context.Context, input, data map[string]interface{}) ([]string, error) {
	// Create an in-memory store with the data
	store := inmem.NewFromObject(data)

	violationQuery, err := rego.New(
		rego.Query("data.cloudformation.violations"),
		rego.Module("cloudformation.rego", policyContent),
		rego.Store(store),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare violations query: %w", err)
	}

	results, err := violationQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate violations: %w", err)
	}

	if len(results) == 0 {
		return []string{"unknown policy violation"}, nil
	}

	violationsInterface := results[0].Expressions[0].Value
	if violationsInterface == nil {
		return []string{"unknown policy violation"}, nil
	}

	// Convert the violations to strings
	var violations []string
	switch v := violationsInterface.(type) {
	case []interface{}:
		for _, violation := range v {
			if str, ok := violation.(string); ok {
				violations = append(violations, str)
			}
		}
	case map[string]interface{}:
		// Handle set type from Rego
		for violation := range v {
			violations = append(violations, violation)
		}
	}

	if len(violations) == 0 {
		return []string{"policy validation failed but no specific violations found"}, nil
	}

	return violations, nil
}
