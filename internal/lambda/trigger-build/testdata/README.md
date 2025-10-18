# Test Data for trigger-build Lambda

This directory contains sample DynamoDB Stream event JSON files used for testing the trigger-build Lambda function.

## Test Files

### insert_event.json

A single INSERT event representing a new build record being added to DynamoDB.

**Content:**

- Single INSERT event
- Repository: `test-repo`
- Environment: `dev`
- Version: `1.0.0-build.123`
- Status: `PENDING`

**Use case:** Testing basic INSERT event handling and unmarshaling of a new build record.

---

### modify_event.json

A single MODIFY event showing a build status update.

**Content:**

- Single MODIFY event
- Shows both OldImage (status: `PENDING`) and NewImage (status: `IN_PROGRESS`)
- Includes execution ARN in NewImage

**Use case:** Testing MODIFY event structure and verifying the Lambda skips non-INSERT events.

---

### multiple_inserts.json

Multiple INSERT events in a single batch.

**Content:**

- Two INSERT events
- Event 1: `frontend/prod` - version `2.5.1-build.456`
- Event 2: `backend/staging` - version `3.0.0-beta.789`

**Use case:** Testing batch processing of multiple INSERT events in one Lambda invocation.

---

### mixed_events.json

A mix of INSERT, MODIFY, and REMOVE events.

**Content:**

- INSERT event: `api/dev` - version `1.2.3-build.999`
- MODIFY event: Status change from `IN_PROGRESS` to `SUCCESS`
- REMOVE event: Deletion of `old-service/dev` record

**Use case:** Testing that the Lambda correctly filters events and only processes INSERT events.

## Event Structure

All events follow the AWS DynamoDB Streams event structure:

```json
{
  "Records": [
    {
      "eventID": "unique-event-id",
      "eventName": "INSERT|MODIFY|REMOVE",
      "eventVersion": "1.1",
      "eventSource": "aws:dynamodb",
      "awsRegion": "us-east-1",
      "dynamodb": {
        "ApproximateCreationDateTime": 1678886400,
        "Keys": {
          "pk": {
            "S": "repo/env"
          },
          "sk": {
            "S": "ksuid"
          }
        },
        "NewImage": {
          /* Full item after change */
        },
        "OldImage": {
          /* Full item before change (for MODIFY/REMOVE) */
        },
        "SequenceNumber": "111",
        "SizeBytes": 26,
        "StreamViewType": "NEW_AND_OLD_IMAGES"
      },
      "eventSourceARN": "arn:aws:dynamodb:..."
    }
  ]
}
```

## DynamoDB Attribute Types

The test files use standard DynamoDB attribute type notation:

- `S` - String
- `N` - Number (stored as string)
- `BOOL` - Boolean
- `NULL` - Null
- `M` - Map
- `L` - List
- `SS` - String Set
- `NS` - Number Set
- `BS` - Binary Set
- `B` - Binary

## Testing Strategy

Tests use `json.Unmarshal` to deserialize these files into `events.DynamoDBEvent` structures, then:

1. Verify the event metadata (eventID, eventName, region, etc.)
2. Convert DynamoDB attribute values to SDK types using `convertDynamoDBAttributeValue()`
3. Unmarshal into `builddao.Record` using `unmarshalMap()`
4. Verify all fields are correctly populated

This approach ensures the Lambda can handle real DynamoDB Stream events exactly as AWS sends them.
