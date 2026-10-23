package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "composebox-auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: Fresh manager, setup required
	m, err := New(tmpDir, "", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if !m.IsSetupRequired() {
		t.Errorf("expected setup to be required on empty dir")
	}

	// Case 2: Setup password
	token, err := m.Setup("secret123")
	if err != nil {
		t.Fatalf("Setup() failed: %v", err)
	}
	if !m.ValidateToken(token) {
		t.Errorf("expected generated token to be valid")
	}
	if m.IsSetupRequired() {
		t.Errorf("setup should not be required after setting password")
	}

	// Case 3: Re-read manager from disk
	m2, err := New(tmpDir, "", false)
	if err != nil {
		t.Fatalf("New() second instance failed: %v", err)
	}
	if m2.IsSetupRequired() {
		t.Errorf("second instance should read saved auth file")
	}

	// Test Login
	loginToken, err := m2.Login("secret123")
	if err != nil {
		t.Fatalf("Login with correct password failed: %v", err)
	}
	if !m2.ValidateToken(loginToken) {
		t.Errorf("expected login token to be valid")
	}

	// Test Invalid Login
	if _, err := m2.Login("wrongpass"); err == nil {
		t.Errorf("expected login with wrong password to fail")
	}

	// Test Tampered Token
	if m2.ValidateToken("12345.tamperedSignature") {
		t.Errorf("expected tampered token to fail validation")
	}

	// Case 4: Env password override
	mEnv, err := New(filepath.Join(tmpDir, "env-test"), "envpassword", false)
	if err != nil {
		t.Fatal(err)
	}
	if mEnv.IsSetupRequired() {
		t.Errorf("setup should not be required when env password is set")
	}
	envToken, err := mEnv.Login("envpassword")
	if err != nil {
		t.Fatalf("Login with env password failed: %v", err)
	}
	if !mEnv.ValidateToken(envToken) {
		t.Errorf("expected env token to be valid")
	}

	// Case 5: Per-IP Brute-force rate limiting
	for i := 0; i < maxFailedAttempts; i++ {
		_, _ = mEnv.Login("badpassword", "10.0.0.1")
	}
	if _, err := mEnv.Login("envpassword", "10.0.0.1"); err == nil {
		t.Errorf("expected IP 10.0.0.1 to be locked out after %d failed attempts", maxFailedAttempts)
	}

	// Another IP (10.0.0.2) must NOT be locked out by 10.0.0.1's failures
	validToken, err := mEnv.Login("envpassword", "10.0.0.2")
	if err != nil {
		t.Fatalf("legitimate IP 10.0.0.2 was blocked by another IP's lockout: %v", err)
	}
	if !mEnv.ValidateToken(validToken) {
		t.Errorf("expected valid token for 10.0.0.2")
	}
}
