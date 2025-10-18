package gql

import (
	"fmt"
	"time"
)

// DateTime represents a custom scalar for time.Time values
type DateTime struct {
	time.Time
}

// ImplementsGraphQLType returns the GraphQL type name
func (DateTime) ImplementsGraphQLType(name string) bool {
	return name == "DateTime"
}

// UnmarshalGraphQL unmarshals a GraphQL DateTime value
func (t *DateTime) UnmarshalGraphQL(input interface{}) error {
	switch input := input.(type) {
	case string:
		parsedTime, err := time.Parse(time.RFC3339, input)
		if err != nil {
			return fmt.Errorf("failed to parse DateTime: %w", err)
		}
		t.Time = parsedTime
		return nil
	case time.Time:
		t.Time = input
		return nil
	default:
		return fmt.Errorf("invalid DateTime type: %T", input)
	}
}

// MarshalJSON marshals DateTime to JSON (RFC3339 format)
func (t DateTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.Format(time.RFC3339) + `"`), nil
}

// NewDateTime creates a new DateTime from a time.Time
func NewDateTime(t time.Time) DateTime {
	return DateTime{Time: t}
}

// NewDateTimePtr creates a new *DateTime from a *time.Time
func NewDateTimePtr(t *time.Time) *DateTime {
	if t == nil {
		return nil
	}
	dt := DateTime{Time: *t}
	return &dt
}

// NewDateTimeFromUnix creates a new DateTime from a Unix timestamp (seconds since epoch)
func NewDateTimeFromUnix(timestamp int64) DateTime {
	return DateTime{Time: time.Unix(timestamp, 0)}
}

// NewDateTimePtrFromUnix creates a new *DateTime from a Unix timestamp pointer
func NewDateTimePtrFromUnix(timestamp *int64) *DateTime {
	if timestamp == nil {
		return nil
	}
	dt := DateTime{Time: time.Unix(*timestamp, 0)}
	return &dt
}
