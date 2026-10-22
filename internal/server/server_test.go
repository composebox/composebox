package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/composebox/composebox/internal/auth"
	"github.com/composebox/composebox/internal/stack"
)

func setupTestServer(t *testing.T) (*Server, string, func()) {
	tmpDir, err := os.MkdirTemp("", "composebox-server-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	authMgr, err := auth.New(tmpDir, "", false)
	if err != nil {
		t.Fatalf("failed to create auth manager: %v", err)
	}

	scan := stack.NewScanner(tmpDir)
	doc := stack.NewDockerClient()
	srv := New(scan, doc, authMgr, "0.1.0")

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return srv, tmpDir, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/health", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff header")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", body["status"])
	}
}

func TestAuthEndpoints(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	handler := srv.Handler()

	// 1. Check initial auth status (setup required)
	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var statusResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &statusResp)
	if statusResp["setup_required"] != true {
		t.Errorf("expected setup_required to be true initially")
	}

	// 2. Access root -> serves Svelte SPA (200 OK)
	rootReq := httptest.NewRequest("GET", "/", nil)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK on root SPA, got %d", rootRec.Code)
	}

	// 3. Access protected API endpoint before auth -> 401
	stacksReq := httptest.NewRequest("GET", "/api/stacks", nil)
	stacksRec := httptest.NewRecorder()
	handler.ServeHTTP(stacksRec, stacksReq)
	if stacksRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for /api/stacks, got %d", stacksRec.Code)
	}

	// 4. Perform setup
	setupPayload := []byte(`{"password":"testadminpassword"}`)
	setupReq := httptest.NewRequest("POST", "/api/auth/setup", bytes.NewReader(setupPayload))
	setupReq.Header.Set("Content-Type", "application/json")
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, setupReq)

	if setupRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on setup, got %d: %s", setupRec.Code, setupRec.Body.String())
	}

	// Extract session cookie
	cookies := setupRec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "composebox_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected session cookie to be set")
	}

	// 5. Access protected endpoint with cookie -> 200 OK
	authStacksReq := httptest.NewRequest("GET", "/api/stacks", nil)
	authStacksReq.AddCookie(sessionCookie)
	authStacksRec := httptest.NewRecorder()
	handler.ServeHTTP(authStacksRec, authStacksReq)
	if authStacksRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /api/stacks after auth, got %d", authStacksRec.Code)
	}

	// 6. Test Stack Creation
	createPayload := []byte(`{"name":"test-api-stack","compose_content":"services:\n  app:\n    image: nginx:alpine\n","env_content":""}`)
	createReq := httptest.NewRequest("POST", "/api/stacks", bytes.NewReader(createPayload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// 7. Get created stack
	getReq := httptest.NewRequest("GET", "/api/stacks/test-api-stack", nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", getRec.Code)
	}

	// 8. Delete created stack
	delReq := httptest.NewRequest("DELETE", "/api/stacks/test-api-stack", nil)
	delReq.AddCookie(sessionCookie)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", delRec.Code)
	}

	// 9. Cross-origin CSRF protection blocks foreign Origin on state-changing requests
	csrfReq := httptest.NewRequest("POST", "/api/stacks", bytes.NewReader(createPayload))
	csrfReq.Host = "localhost:8888"
	csrfReq.Header.Set("Origin", "https://evil.example.com")
	csrfReq.AddCookie(sessionCookie)
	csrfRec := httptest.NewRecorder()
	handler.ServeHTTP(csrfRec, csrfReq)
	if csrfRec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for cross-origin POST, got %d", csrfRec.Code)
	}
}
