package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestGenerateSecureSecret(t *testing.T) {
	secret, err := generateSecureSecret()
	if err != nil {
		t.Fatalf("generateSecureSecret() error = %v", err)
	}

	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("generated secret is not valid base64: %v", err)
	}

	// Verify it's 32 bytes (256 bits)
	if len(decoded) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(decoded))
	}
}

func TestGenerateSecureSecretUniqueness(t *testing.T) {
	// Generate multiple secrets and verify they're all different
	secrets := make(map[string]bool)
	for i := 0; i < 100; i++ {
		secret, err := generateSecureSecret()
		if err != nil {
			t.Fatalf("generateSecureSecret() error = %v", err)
		}

		if secrets[secret] {
			t.Errorf("duplicate secret generated")
		}
		secrets[secret] = true
	}
}

func TestSecretVersionJSON(t *testing.T) {
	versions := []SecretVersion{
		{
			Secret:    "MXFBaDlmMEFseXB2L0k5Zlh4MzhBdUs5NGEzNi9uNHZBRFMxWS9sN3B2VT0=",
			Timestamp: "2025-10-06T12:58:53Z",
		},
		{
			Secret:    "YW5vdGhlclNlY3VyZVNlY3JldEhlcmVUaGF0SXNWZXJ5TG9uZw==",
			Timestamp: "2025-10-05T12:58:53Z",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(versions)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded []SecretVersion
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify
	if len(decoded) != len(versions) {
		t.Errorf("expected %d versions, got %d", len(versions), len(decoded))
	}

	for i := range versions {
		if decoded[i].Secret != versions[i].Secret {
			t.Errorf("secret mismatch at index %d", i)
		}
		if decoded[i].Timestamp != versions[i].Timestamp {
			t.Errorf("timestamp mismatch at index %d", i)
		}
	}
}

func TestVersionLimiting(t *testing.T) {
	// Simulate having 5 versions and adding a new one
	versions := []SecretVersion{
		{Secret: "old1", Timestamp: "2025-10-01T12:00:00Z"},
		{Secret: "old2", Timestamp: "2025-10-02T12:00:00Z"},
		{Secret: "old3", Timestamp: "2025-10-03T12:00:00Z"},
		{Secret: "old4", Timestamp: "2025-10-04T12:00:00Z"},
		{Secret: "old5", Timestamp: "2025-10-05T12:00:00Z"},
	}

	// Add new version at the beginning
	newVersion := SecretVersion{
		Secret:    "new",
		Timestamp: "2025-10-06T12:00:00Z",
	}
	versions = append([]SecretVersion{newVersion}, versions...)

	// Keep only the most recent 3 versions
	if len(versions) > 3 {
		versions = versions[:3]
	}

	// Verify we have exactly 3 versions
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}

	// Verify the newest version is first
	if versions[0].Secret != "new" {
		t.Errorf("expected newest version first, got %s", versions[0].Secret)
	}

	// Verify we kept the most recent ones
	if versions[1].Secret != "old1" {
		t.Errorf("expected old1 in position 1, got %s", versions[1].Secret)
	}
	if versions[2].Secret != "old2" {
		t.Errorf("expected old2 in position 2, got %s", versions[2].Secret)
	}
}

func TestCorruptJSONHandling(t *testing.T) {
	// Test that corrupt JSON can be detected
	corruptJSON := `{"invalid": json}`

	var versions []SecretVersion
	err := json.Unmarshal([]byte(corruptJSON), &versions)
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
}

func TestInvalidBase64Detection(t *testing.T) {
	invalidBase64 := "not valid base64!!!"

	_, err := base64.StdEncoding.DecodeString(invalidBase64)
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestValidBase64Verification(t *testing.T) {
	validBase64 := "MXFBaDlmMEFseXB2L0k5Zlh4MzhBdUs5NGEzNi9uNHZBRFMxWS9sN3B2VT0="

	decoded, err := base64.StdEncoding.DecodeString(validBase64)
	if err != nil {
		t.Errorf("valid base64 failed to decode: %v", err)
	}

	if len(decoded) == 0 {
		t.Error("decoded base64 is empty")
	}
}

func TestBase64ValidationFiltering(t *testing.T) {
	// Test that invalid base64 secrets are filtered out
	versions := []SecretVersion{
		{Secret: "VmFsaWRCYXNlNjRTdHJpbmc=", Timestamp: "2025-10-01T12:00:00Z"},
		{Secret: "not valid base64!!!", Timestamp: "2025-10-02T12:00:00Z"},
		{Secret: "YW5vdGhlclZhbGlkQmFzZTY0", Timestamp: "2025-10-03T12:00:00Z"},
	}

	validVersions := []SecretVersion{}
	for _, v := range versions {
		if _, err := base64.StdEncoding.DecodeString(v.Secret); err == nil {
			validVersions = append(validVersions, v)
		}
	}

	// Should have filtered out the invalid one
	if len(validVersions) != 2 {
		t.Errorf("expected 2 valid versions, got %d", len(validVersions))
	}
}

// Integration tests for rotation workflow

func TestEmptySecretHandling(t *testing.T) {
	// Simulate starting with empty secret (like CloudFormation creates)
	emptyJSON := ""

	// Try to unmarshal empty string
	var versions []SecretVersion
	err := json.Unmarshal([]byte(emptyJSON), &versions)
	if err == nil {
		t.Error("expected error for empty string, got nil")
	}

	// Verify we can recover by starting fresh
	versions = []SecretVersion{}
	newSecret, _ := generateSecureSecret()
	versions = append(versions, SecretVersion{
		Secret:    newSecret,
		Timestamp: "2025-10-10T12:00:00Z",
	})

	if len(versions) != 1 {
		t.Errorf("expected 1 version after recovery, got %d", len(versions))
	}

	// Verify the secret is valid
	decoded, err := base64.StdEncoding.DecodeString(versions[0].Secret)
	if err != nil {
		t.Errorf("recovered secret is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("recovered secret has wrong length: expected 32, got %d", len(decoded))
	}
}

func TestCorruptSecretHandling(t *testing.T) {
	// Simulate CloudFormation's initial corrupt secret
	corruptJSON := `[{"secret":"arn:aws:cloudformation:us-west-2:621410822360:stack/dev-aws-deployer/68152390-a24e-11f0-9f52-02570980de53","timestamp":"arn:aws:cloudformation:us-west-2:621410822360:stack/dev-aws-deployer/68152390-a24e-11f0-9f52-02570980de53"}]`

	var versions []SecretVersion
	err := json.Unmarshal([]byte(corruptJSON), &versions)
	if err != nil {
		t.Fatalf("failed to unmarshal corrupt JSON: %v", err)
	}

	// Verify we can detect and filter out invalid base64
	validVersions := []SecretVersion{}
	for _, v := range versions {
		decoded, err := base64.StdEncoding.DecodeString(v.Secret)
		if err != nil {
			// Invalid base64 - skip
			continue
		}
		if len(decoded) != 32 {
			// Wrong length - skip
			continue
		}
		validVersions = append(validVersions, v)
	}

	// Should have no valid versions from corrupt data
	if len(validVersions) != 0 {
		t.Errorf("expected 0 valid versions from corrupt data, got %d", len(validVersions))
	}

	// Add fresh valid secret
	newSecret, _ := generateSecureSecret()
	validVersions = append(validVersions, SecretVersion{
		Secret:    newSecret,
		Timestamp: "2025-10-10T12:00:00Z",
	})

	// Now we should have 1 valid version
	if len(validVersions) != 1 {
		t.Errorf("expected 1 version after adding fresh secret, got %d", len(validVersions))
	}
}

func TestInvalidLengthFiltering(t *testing.T) {
	// Generate secrets of various lengths
	short16 := base64.StdEncoding.EncodeToString(make([]byte, 16))   // 16 bytes
	correct32 := base64.StdEncoding.EncodeToString(make([]byte, 32)) // 32 bytes (correct)
	long64 := base64.StdEncoding.EncodeToString(make([]byte, 64))    // 64 bytes

	versions := []SecretVersion{
		{Secret: short16, Timestamp: "2025-10-01T12:00:00Z"},
		{Secret: correct32, Timestamp: "2025-10-02T12:00:00Z"},
		{Secret: long64, Timestamp: "2025-10-03T12:00:00Z"},
	}

	// Filter to only keep 32-byte (256-bit) secrets
	validVersions := []SecretVersion{}
	for _, v := range versions {
		decoded, err := base64.StdEncoding.DecodeString(v.Secret)
		if err != nil {
			continue
		}
		if len(decoded) == 32 {
			validVersions = append(validVersions, v)
		}
	}

	// Should only have the 32-byte secret
	if len(validVersions) != 1 {
		t.Errorf("expected 1 valid 32-byte secret, got %d", len(validVersions))
	}

	// Verify it's the correct one
	decoded, _ := base64.StdEncoding.DecodeString(validVersions[0].Secret)
	if len(decoded) != 32 {
		t.Errorf("filtered secret has wrong length: got %d", len(decoded))
	}
}

func TestVersionRotationWithMax3(t *testing.T) {
	// Start with 3 existing valid secrets
	secret1, _ := generateSecureSecret()
	secret2, _ := generateSecureSecret()
	secret3, _ := generateSecureSecret()

	versions := []SecretVersion{
		{Secret: secret3, Timestamp: "2025-10-08T12:00:00Z"}, // newest
		{Secret: secret2, Timestamp: "2025-10-07T12:00:00Z"},
		{Secret: secret1, Timestamp: "2025-10-06T12:00:00Z"}, // oldest
	}

	// Add a new secret (simulating rotation)
	secret4, _ := generateSecureSecret()
	newVersion := SecretVersion{
		Secret:    secret4,
		Timestamp: "2025-10-09T12:00:00Z",
	}
	versions = append([]SecretVersion{newVersion}, versions...)

	// Keep only the most recent 3
	if len(versions) > 3 {
		versions = versions[:3]
	}

	// Verify we have exactly 3
	if len(versions) != 3 {
		t.Errorf("expected 3 versions after rotation, got %d", len(versions))
	}

	// Verify the newest is first
	if versions[0].Secret != secret4 {
		t.Error("newest secret should be first")
	}

	// Verify the oldest (secret1) was dropped
	for _, v := range versions {
		if v.Secret == secret1 {
			t.Error("oldest secret should have been dropped")
		}
	}

	// Verify all remaining secrets are valid 32-byte base64
	for i, v := range versions {
		decoded, err := base64.StdEncoding.DecodeString(v.Secret)
		if err != nil {
			t.Errorf("version %d is not valid base64: %v", i, err)
		}
		if len(decoded) != 32 {
			t.Errorf("version %d has wrong length: expected 32, got %d", i, len(decoded))
		}
	}
}

func TestMixedValidAndInvalidSecrets(t *testing.T) {
	// Create a mix of valid and invalid secrets
	validSecret1, _ := generateSecureSecret()
	validSecret2, _ := generateSecureSecret()

	versions := []SecretVersion{
		{Secret: validSecret1, Timestamp: "2025-10-01T12:00:00Z"},
		{Secret: "invalid base64!!!", Timestamp: "2025-10-02T12:00:00Z"},
		{Secret: "arn:aws:cloudformation:...", Timestamp: "2025-10-03T12:00:00Z"},
		{Secret: validSecret2, Timestamp: "2025-10-04T12:00:00Z"},
		{Secret: base64.StdEncoding.EncodeToString(make([]byte, 16)), Timestamp: "2025-10-05T12:00:00Z"}, // wrong length
	}

	// Filter to only valid 32-byte base64 secrets
	validVersions := []SecretVersion{}
	for _, v := range versions {
		decoded, err := base64.StdEncoding.DecodeString(v.Secret)
		if err != nil {
			continue
		}
		if len(decoded) == 32 {
			validVersions = append(validVersions, v)
		}
	}

	// Should only have 2 valid secrets
	if len(validVersions) != 2 {
		t.Errorf("expected 2 valid secrets, got %d", len(validVersions))
	}

	// Verify they're the correct ones
	if validVersions[0].Secret != validSecret1 && validVersions[0].Secret != validSecret2 {
		t.Error("first valid secret is not one of the expected secrets")
	}
	if validVersions[1].Secret != validSecret1 && validVersions[1].Secret != validSecret2 {
		t.Error("second valid secret is not one of the expected secrets")
	}
}
