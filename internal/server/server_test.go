package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	srv := New(scan, doc, authMgr, "0.1.0", "testcommit")

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
	setupReq.Header.Set("X-Composebox-Client", "true")
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
	createReq.Header.Set("X-Composebox-Client", "true")
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
	delReq.Header.Set("X-Composebox-Client", "true")
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
	csrfReq.Header.Set("X-Composebox-Client", "true")
	csrfReq.AddCookie(sessionCookie)
	csrfRec := httptest.NewRecorder()
	handler.ServeHTTP(csrfRec, csrfReq)
	if csrfRec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for cross-origin POST, got %d", csrfRec.Code)
	}

	// 10. Missing X-Composebox-Client header on mutation request blocks CSRF
	noHeaderReq := httptest.NewRequest("POST", "/api/stacks", bytes.NewReader(createPayload))
	noHeaderReq.Header.Set("Content-Type", "application/json")
	noHeaderReq.AddCookie(sessionCookie)
	noHeaderRec := httptest.NewRecorder()
	handler.ServeHTTP(noHeaderRec, noHeaderReq)
	if noHeaderRec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for missing X-Composebox-Client header, got %d", noHeaderRec.Code)
	}

	// 11. Logout revokes token server-side
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.Header.Set("X-Composebox-Client", "true")
	logoutReq.AddCookie(sessionCookie)
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on logout, got %d", logoutRec.Code)
	}

	// Reusing session cookie after logout must fail with 401 Unauthorized
	postLogoutReq := httptest.NewRequest("GET", "/api/stacks", nil)
	postLogoutReq.AddCookie(sessionCookie)
	postLogoutRec := httptest.NewRecorder()
	handler.ServeHTTP(postLogoutRec, postLogoutReq)
	if postLogoutRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized after logout revocation, got %d", postLogoutRec.Code)
	}
}

func TestIsSameHost(t *testing.T) {
	cases := []struct {
		origin  string
		request string
		want    bool
	}{
		{"localhost:8080", "localhost:8080", true},
		{"localhost:8080", "localhost:9090", false},
		{"compose.skp.my.id", "compose.skp.my.id", true},
		{"compose.skp.my.id:443", "compose.skp.my.id", true},
		{"compose.skp.my.id", "compose.skp.my.id:443", true},
		{"compose.skp.my.id:80", "compose.skp.my.id", true},
		{"compose.skp.my.id:8080", "compose.skp.my.id", false},
		{"evil.com:8080", "localhost:8080", false},
	}

	for _, tc := range cases {
		got := isSameHost(tc.origin, tc.request)
		if got != tc.want {
			t.Errorf("isSameHost(%q, %q) = %v, want %v", tc.origin, tc.request, got, tc.want)
		}
	}
}

func TestGetClientIP_TrustedProxies(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Untrusted caller supplying spoofed headers -> must ignore headers and return remote host
	reqUntrusted := httptest.NewRequest("GET", "/api/health", nil)
	reqUntrusted.RemoteAddr = "198.51.100.5:12345"
	reqUntrusted.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqUntrusted.Header.Set("CF-Connecting-IP", "203.0.113.199")

	ipUntrusted := srv.getClientIP(reqUntrusted)
	if ipUntrusted != "198.51.100.5" {
		t.Errorf("expected untrusted IP 198.51.100.5, got %s", ipUntrusted)
	}

	// 2. Trusted caller (127.0.0.1) supplying CF-Connecting-IP -> trust header
	reqTrustedCF := httptest.NewRequest("GET", "/api/health", nil)
	reqTrustedCF.RemoteAddr = "127.0.0.1:54321"
	reqTrustedCF.Header.Set("CF-Connecting-IP", "203.0.113.50")

	ipTrustedCF := srv.getClientIP(reqTrustedCF)
	if ipTrustedCF != "203.0.113.50" {
		t.Errorf("expected trusted CF-Connecting-IP 203.0.113.50, got %s", ipTrustedCF)
	}

	// 3. Trusted caller supplying X-Forwarded-For -> trust first entry
	reqTrustedXFF := httptest.NewRequest("GET", "/api/health", nil)
	reqTrustedXFF.RemoteAddr = "127.0.0.1:54321"
	reqTrustedXFF.Header.Set("X-Forwarded-For", "198.51.100.88, 10.0.0.1")

	ipTrustedXFF := srv.getClientIP(reqTrustedXFF)
	if ipTrustedXFF != "198.51.100.88" {
		t.Errorf("expected trusted X-Forwarded-For 198.51.100.88, got %s", ipTrustedXFF)
	}
}

func TestSystemEndpoints(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a test stack with ports
	composeYaml := "services:\n  app:\n    image: nginx\n    ports:\n      - \"127.0.0.1:9099:80\"\n"
	if err := srv.scanner.CreateStack("port-test", composeYaml, ""); err != nil {
		t.Fatalf("failed to create stack: %v", err)
	}

	// 1. Authenticate session
	token, _ := srv.authManager.Setup("adminpassword")
	cookie := &http.Cookie{Name: "composebox_session", Value: token}

	// 2. Query /api/system/ports
	portsReq := httptest.NewRequest("GET", "/api/system/ports", nil)
	portsReq.AddCookie(cookie)
	portsRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(portsRec, portsReq)

	if portsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/system/ports, got %d: %s", portsRec.Code, portsRec.Body.String())
	}

	var portsResp struct {
		Allocations []stack.PortAllocation `json:"allocations"`
	}
	if err := json.Unmarshal(portsRec.Body.Bytes(), &portsResp); err != nil {
		t.Fatalf("failed to unmarshal ports response: %v", err)
	}

	found := false
	for _, a := range portsResp.Allocations {
		if a.Port == 9099 && a.Stack == "port-test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find port 9099 for stack 'port-test'")
	}

	// 3. Test POST /api/system/prune-item invalid requests
	badItemReq := httptest.NewRequest("POST", "/api/system/prune-item", strings.NewReader(`{"type":"invalid","id":"123"}`))
	badItemReq.AddCookie(cookie)
	badItemReq.Header.Set("Content-Type", "application/json")
	badItemReq.Header.Set("X-Composebox-Client", "true")
	badItemRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badItemRec, badItemReq)
	if badItemRec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid item type, got %d", badItemRec.Code)
	}

	// 4. Test POST /api/stacks/port-test/stop endpoint registration
	stopReq := httptest.NewRequest("POST", "/api/stacks/port-test/stop", nil)
	stopReq.AddCookie(cookie)
	stopReq.Header.Set("X-Composebox-Client", "true")
	stopRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(stopRec, stopReq)
	// Stop endpoint is routed and responds (either 200 or 500 depending on docker availability in test env)
	if stopRec.Code != http.StatusOK && stopRec.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500 from stop handler, got %d: %s", stopRec.Code, stopRec.Body.String())
	}
}
