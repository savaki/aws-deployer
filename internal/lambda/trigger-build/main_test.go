package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
)

func TestConvertDynamoDBAttributeValue_String(t *testing.T) {
	av := events.NewStringAttribute("test-value")
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberS)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberS, got %T", result)
	}

	if member.Value != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", member.Value)
	}
}

func TestConvertDynamoDBAttributeValue_Number(t *testing.T) {
	av := events.NewNumberAttribute("42")
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberN)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberN, got %T", result)
	}

	if member.Value != "42" {
		t.Errorf("Expected '42', got '%s'", member.Value)
	}
}

func TestConvertDynamoDBAttributeValue_Boolean(t *testing.T) {
	av := events.NewBooleanAttribute(true)
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberBOOL)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberBOOL, got %T", result)
	}

	if !member.Value {
		t.Errorf("Expected true, got false")
	}
}

func TestConvertDynamoDBAttributeValue_Null(t *testing.T) {
	av := events.NewNullAttribute()
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberNULL)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberNULL, got %T", result)
	}

	if !member.Value {
		t.Errorf("Expected true, got false")
	}
}

func TestConvertDynamoDBAttributeValue_StringSet(t *testing.T) {
	av := events.NewStringSetAttribute([]string{"a", "b", "c"})
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberSS)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberSS, got %T", result)
	}

	if len(member.Value) != 3 {
		t.Errorf("Expected length 3, got %d", len(member.Value))
	}

	if member.Value[0] != "a" || member.Value[1] != "b" || member.Value[2] != "c" {
		t.Errorf("Expected [a, b, c], got %v", member.Value)
	}
}

func TestConvertDynamoDBAttributeValue_NumberSet(t *testing.T) {
	av := events.NewNumberSetAttribute([]string{"1", "2", "3"})
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberNS)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberNS, got %T", result)
	}

	if len(member.Value) != 3 {
		t.Errorf("Expected length 3, got %d", len(member.Value))
	}

	if member.Value[0] != "1" || member.Value[1] != "2" || member.Value[2] != "3" {
		t.Errorf("Expected [1, 2, 3], got %v", member.Value)
	}
}

func TestConvertDynamoDBAttributeValue_List(t *testing.T) {
	list := []events.DynamoDBAttributeValue{
		events.NewStringAttribute("item1"),
		events.NewStringAttribute("item2"),
	}
	av := events.NewListAttribute(list)
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberL)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberL, got %T", result)
	}

	if len(member.Value) != 2 {
		t.Errorf("Expected length 2, got %d", len(member.Value))
	}

	item1, ok := member.Value[0].(*types.AttributeValueMemberS)
	if !ok || item1.Value != "item1" {
		t.Errorf("Expected first item to be 'item1'")
	}

	item2, ok := member.Value[1].(*types.AttributeValueMemberS)
	if !ok || item2.Value != "item2" {
		t.Errorf("Expected second item to be 'item2'")
	}
}

func TestConvertDynamoDBAttributeValue_Map(t *testing.T) {
	mapVal := map[string]events.DynamoDBAttributeValue{
		"key1": events.NewStringAttribute("value1"),
		"key2": events.NewNumberAttribute("123"),
	}
	av := events.NewMapAttribute(mapVal)
	result := convertDynamoDBAttributeValue(av)

	member, ok := result.(*types.AttributeValueMemberM)
	if !ok {
		t.Fatalf("Expected *types.AttributeValueMemberM, got %T", result)
	}

	if len(member.Value) != 2 {
		t.Errorf("Expected length 2, got %d", len(member.Value))
	}

	key1Val, ok := member.Value["key1"].(*types.AttributeValueMemberS)
	if !ok || key1Val.Value != "value1" {
		t.Errorf("Expected key1 to be 'value1'")
	}

	key2Val, ok := member.Value["key2"].(*types.AttributeValueMemberN)
	if !ok || key2Val.Value != "123" {
		t.Errorf("Expected key2 to be '123'")
	}
}

func TestUnmarshalMap(t *testing.T) {
	// Create a mock DynamoDB record similar to what would come from a stream
	m := map[string]types.AttributeValue{
		"pk":         &types.AttributeValueMemberS{Value: "test-repo/dev"},
		"sk":         &types.AttributeValueMemberS{Value: "2HFj3kLmNoPqRsTuVwXy"},
		"repo":       &types.AttributeValueMemberS{Value: "test-repo"},
		"env":        &types.AttributeValueMemberS{Value: "dev"},
		"branch":     &types.AttributeValueMemberS{Value: "main"},
		"version":    &types.AttributeValueMemberS{Value: "1.0.0"},
		"commitHash": &types.AttributeValueMemberS{Value: "abc123def456"},
	}

	var record struct {
		PK         string
		SK         string
		Repo       string
		Env        string
		Branch     string
		Version    string
		CommitHash string
	}

	// Note: unmarshalMap currently only supports builddao.Record type
	// This test documents the current limitation
	err := unmarshalMap(m, &record)
	if err == nil {
		t.Error("Expected error for unsupported type, got nil")
	}
}

func TestHandleDynamoDBEvent_EmptyRecords(t *testing.T) {
	handler := &Handler{
		singleAccountOrchestrator: nil, // nil is okay for this test since we won't process records
		multiAccountOrchestrator:  nil,
		dao:                       nil,
		targetDAO:                 nil,
	}

	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{},
	}

	err := handler.HandleDynamoDBEvent(context.Background(), event)
	if err != nil {
		t.Errorf("Expected no error for empty records, got %v", err)
	}
}

func TestHandleDynamoDBEvent_SkipNonInsert(t *testing.T) {
	handler := &Handler{
		singleAccountOrchestrator: nil,
		multiAccountOrchestrator:  nil,
		dao:                       nil,
		targetDAO:                 nil,
	}

	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "MODIFY",
				EventID:   "test-event-id",
			},
			{
				EventName: "REMOVE",
				EventID:   "test-event-id-2",
			},
		},
	}

	err := handler.HandleDynamoDBEvent(context.Background(), event)
	if err != nil {
		t.Errorf("Expected no error for non-INSERT events, got %v", err)
	}
}

// loadTestEvent loads a DynamoDB event from a JSON file in testdata
func loadTestEvent(t *testing.T, filename string) events.DynamoDBEvent {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", path, err)
	}

	var event events.DynamoDBEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("Failed to unmarshal event from %s: %v", path, err)
	}

	return event
}

func TestUnmarshalInsertEvent(t *testing.T) {
	event := loadTestEvent(t, "insert_event.json")

	if len(event.Records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(event.Records))
	}

	record := event.Records[0]

	// Verify event metadata
	if record.EventID != "1" {
		t.Errorf("Expected EventID '1', got '%s'", record.EventID)
	}
	if record.EventName != "INSERT" {
		t.Errorf("Expected EventName 'INSERT', got '%s'", record.EventName)
	}
	if record.AWSRegion != "us-east-1" {
		t.Errorf("Expected AWSRegion 'us-east-1', got '%s'", record.AWSRegion)
	}

	// Verify Keys
	if len(record.Change.Keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(record.Change.Keys))
	}

	pkKey := record.Change.Keys["pk"]
	if pkKey.DataType() != events.DataTypeString {
		t.Errorf("Expected pk to be String type")
	}
	if pkKey.String() != "test-repo/dev" {
		t.Errorf("Expected pk 'test-repo/dev', got '%s'", pkKey.String())
	}

	// Verify NewImage can be converted and unmarshaled
	newImage := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		newImage[k] = convertDynamoDBAttributeValue(v)
	}

	var buildRecord builddao.Record
	if err := unmarshalMap(newImage, &buildRecord); err != nil {
		t.Fatalf("Failed to unmarshal build record: %v", err)
	}

	// Verify unmarshaled values
	if buildRecord.PK != "test-repo/dev" {
		t.Errorf("Expected PK 'test-repo/dev', got '%s'", buildRecord.PK)
	}
	if buildRecord.SK != "2HFj3kLmNoPqRsTuVwXy" {
		t.Errorf("Expected SK '2HFj3kLmNoPqRsTuVwXy', got '%s'", buildRecord.SK)
	}
	if buildRecord.Repo != "test-repo" {
		t.Errorf("Expected Repo 'test-repo', got '%s'", buildRecord.Repo)
	}
	if buildRecord.Env != "dev" {
		t.Errorf("Expected Env 'dev', got '%s'", buildRecord.Env)
	}
	if buildRecord.Branch != "main" {
		t.Errorf("Expected Branch 'main', got '%s'", buildRecord.Branch)
	}
	if buildRecord.Version != "1.0.0-build.123" {
		t.Errorf("Expected Version '1.0.0-build.123', got '%s'", buildRecord.Version)
	}
	if buildRecord.CommitHash != "abc123def456789" {
		t.Errorf("Expected CommitHash 'abc123def456789', got '%s'", buildRecord.CommitHash)
	}
}

func TestUnmarshalModifyEvent(t *testing.T) {
	event := loadTestEvent(t, "modify_event.json")

	if len(event.Records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(event.Records))
	}

	record := event.Records[0]

	// Verify this is a MODIFY event
	if record.EventName != "MODIFY" {
		t.Errorf("Expected EventName 'MODIFY', got '%s'", record.EventName)
	}

	// Verify both NewImage and OldImage exist
	if len(record.Change.NewImage) == 0 {
		t.Error("Expected NewImage to be present")
	}
	if len(record.Change.OldImage) == 0 {
		t.Error("Expected OldImage to be present")
	}

	// Verify NewImage has updated status
	statusAttr := record.Change.NewImage["status"]
	if statusAttr.String() != "IN_PROGRESS" {
		t.Errorf("Expected NewImage status 'IN_PROGRESS', got '%s'", statusAttr.String())
	}

	// Verify OldImage has old status
	oldStatusAttr := record.Change.OldImage["status"]
	if oldStatusAttr.String() != "PENDING" {
		t.Errorf("Expected OldImage status 'PENDING', got '%s'", oldStatusAttr.String())
	}
}

func TestUnmarshalMultipleInserts(t *testing.T) {
	event := loadTestEvent(t, "multiple_inserts.json")

	if len(event.Records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(event.Records))
	}

	// First record - frontend/prod
	record1 := event.Records[0]
	if record1.EventName != "INSERT" {
		t.Errorf("Expected EventName 'INSERT' for record 1, got '%s'", record1.EventName)
	}

	newImage1 := make(map[string]types.AttributeValue)
	for k, v := range record1.Change.NewImage {
		newImage1[k] = convertDynamoDBAttributeValue(v)
	}

	var build1 builddao.Record
	if err := unmarshalMap(newImage1, &build1); err != nil {
		t.Fatalf("Failed to unmarshal build record 1: %v", err)
	}

	if build1.Repo != "frontend" {
		t.Errorf("Expected Repo 'frontend' for record 1, got '%s'", build1.Repo)
	}
	if build1.Env != "prod" {
		t.Errorf("Expected Env 'prod' for record 1, got '%s'", build1.Env)
	}
	if build1.Version != "2.5.1-build.456" {
		t.Errorf("Expected Version '2.5.1-build.456' for record 1, got '%s'", build1.Version)
	}

	// Second record - backend/staging
	record2 := event.Records[1]
	if record2.EventName != "INSERT" {
		t.Errorf("Expected EventName 'INSERT' for record 2, got '%s'", record2.EventName)
	}

	newImage2 := make(map[string]types.AttributeValue)
	for k, v := range record2.Change.NewImage {
		newImage2[k] = convertDynamoDBAttributeValue(v)
	}

	var build2 builddao.Record
	if err := unmarshalMap(newImage2, &build2); err != nil {
		t.Fatalf("Failed to unmarshal build record 2: %v", err)
	}

	if build2.Repo != "backend" {
		t.Errorf("Expected Repo 'backend' for record 2, got '%s'", build2.Repo)
	}
	if build2.Env != "staging" {
		t.Errorf("Expected Env 'staging' for record 2, got '%s'", build2.Env)
	}
	if build2.Version != "3.0.0-beta.789" {
		t.Errorf("Expected Version '3.0.0-beta.789' for record 2, got '%s'", build2.Version)
	}
}

func TestUnmarshalMixedEvents(t *testing.T) {
	event := loadTestEvent(t, "mixed_events.json")

	if len(event.Records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(event.Records))
	}

	// Count event types
	var insertCount, modifyCount, removeCount int
	for _, record := range event.Records {
		switch record.EventName {
		case "INSERT":
			insertCount++
		case "MODIFY":
			modifyCount++
		case "REMOVE":
			removeCount++
		}
	}

	if insertCount != 1 {
		t.Errorf("Expected 1 INSERT event, got %d", insertCount)
	}
	if modifyCount != 1 {
		t.Errorf("Expected 1 MODIFY event, got %d", modifyCount)
	}
	if removeCount != 1 {
		t.Errorf("Expected 1 REMOVE event, got %d", removeCount)
	}

	// Test that REMOVE event has no NewImage but has OldImage
	removeRecord := event.Records[2]
	if removeRecord.EventName != "REMOVE" {
		t.Fatalf("Expected third record to be REMOVE")
	}

	if len(removeRecord.Change.NewImage) != 0 {
		t.Error("Expected REMOVE event to have no NewImage")
	}

	if len(removeRecord.Change.OldImage) == 0 {
		t.Error("Expected REMOVE event to have OldImage")
	}
}

func TestHandleDynamoDBEvent_WithRealJSON(t *testing.T) {
	// Test that the handler can process real JSON events without errors
	// We won't test the full orchestration logic, just that unmarshaling works

	// Test with insert event
	event := loadTestEvent(t, "insert_event.json")

	// We expect this to fail at the orchestration step, not at unmarshaling
	// Since orchestrator is nil, processRecord will panic
	// Instead, let's just verify we can process the records up to that point

	for i := range event.Records {
		record := &event.Records[i]

		if record.EventName != "INSERT" {
			continue
		}

		// Convert and unmarshal NewImage
		newImage := make(map[string]types.AttributeValue)
		for k, v := range record.Change.NewImage {
			newImage[k] = convertDynamoDBAttributeValue(v)
		}

		var buildRecord builddao.Record
		if err := unmarshalMap(newImage, &buildRecord); err != nil {
			t.Errorf("Failed to unmarshal build record: %v", err)
		}

		// Verify we got valid data
		if buildRecord.Repo == "" {
			t.Error("Expected Repo to be non-empty")
		}
		if buildRecord.Env == "" {
			t.Error("Expected Env to be non-empty")
		}
		if buildRecord.Version == "" {
			t.Error("Expected Version to be non-empty")
		}
	}
}

func TestHandleDynamoDBEvent_SkipsNonInsertFromRealJSON(t *testing.T) {
	// Load mixed events which contains MODIFY and REMOVE
	event := loadTestEvent(t, "mixed_events.json")

	// Since only INSERT events trigger processing and we have nil orchestrator,
	// this should skip MODIFY and REMOVE without error
	// The INSERT will try to process but fail at orchestration
	// We can't test the full flow without mocking, but we verify unmarshaling works

	for i := range event.Records {
		record := &event.Records[i]

		if record.EventName != "INSERT" {
			continue
		}

		// This should successfully unmarshal
		newImage := make(map[string]types.AttributeValue)
		for k, v := range record.Change.NewImage {
			newImage[k] = convertDynamoDBAttributeValue(v)
		}

		var buildRecord builddao.Record
		if err := unmarshalMap(newImage, &buildRecord); err != nil {
			t.Errorf("Failed to unmarshal build record from mixed events: %v", err)
		}

		if buildRecord.Repo != "api" {
			t.Errorf("Expected Repo 'api', got '%s'", buildRecord.Repo)
		}
	}
}
