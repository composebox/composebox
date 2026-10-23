package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/composebox/composebox/internal/auth"
	"github.com/composebox/composebox/internal/stack"
	"github.com/composebox/composebox/ui"
)

// Server handles HTTP API requests and serves embedded UI assets.
type Server struct {
	scanner        *stack.Scanner
	dockerClient   *stack.DockerClient
	authManager    *auth.Manager
	mux            *http.ServeMux
	version        string
	trustedProxies []*net.IPNet
	trustedIPs     []net.IP
}

// New creates a new HTTP server.
func New(s *stack.Scanner, d *stack.DockerClient, a *auth.Manager, version string, trustedProxiesStr ...string) *Server {
	if version == "" {
		version = "dev"
	}
	srv := &Server{
		scanner:      s,
		dockerClient: d,
		authManager:  a,
		mux:          http.NewServeMux(),
		version:      version,
	}

	var proxyList string
	if len(trustedProxiesStr) > 0 && trustedProxiesStr[0] != "" {
		proxyList = trustedProxiesStr[0]
	} else {
		proxyList = "127.0.0.1,::1"
	}
	srv.parseTrustedProxies(proxyList)

	srv.routes()
	return srv
}

func (s *Server) parseTrustedProxies(raw string) {
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(entry); err == nil {
			s.trustedProxies = append(s.trustedProxies, ipNet)
		} else if ip := net.ParseIP(entry); ip != nil {
			s.trustedIPs = append(s.trustedIPs, ip)
		}
	}
}

func (s *Server) isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, tip := range s.trustedIPs {
		if tip.Equal(ip) {
			return true
		}
	}
	for _, tnet := range s.trustedProxies {
		if tnet.Contains(ip) {
			return true
		}
	}
	return false
}

// Handler returns the HTTP handler with security headers, CSRF validation, and logging configured.
func (s *Server) Handler() http.Handler {
	return s.loggingMiddleware(s.securityMiddleware(s.mux))
}

func (s *Server) routes() {
	// Public authentication & health routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	s.mux.HandleFunc("POST /api/auth/setup", s.handleAuthSetup)
	s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)

	// Protected routes
	s.mux.Handle("GET /api/stacks", s.authManager.Middleware(http.HandlerFunc(s.handleListStacks)))
	s.mux.Handle("POST /api/stacks", s.authManager.Middleware(http.HandlerFunc(s.handleCreateStack)))
	s.mux.Handle("GET /api/stacks/{name}", s.authManager.Middleware(http.HandlerFunc(s.handleGetStack)))
	s.mux.Handle("POST /api/stacks/{name}/save", s.authManager.Middleware(http.HandlerFunc(s.handleSaveStack)))
	s.mux.Handle("DELETE /api/stacks/{name}", s.authManager.Middleware(http.HandlerFunc(s.handleDeleteStack)))
	s.mux.Handle("POST /api/stacks/{name}/up", s.authManager.Middleware(s.handleStackAction("up")))
	s.mux.Handle("POST /api/stacks/{name}/down", s.authManager.Middleware(s.handleStackAction("down")))
	s.mux.Handle("POST /api/stacks/{name}/restart", s.authManager.Middleware(s.handleStackAction("restart")))
	s.mux.Handle("POST /api/stacks/{name}/pull", s.authManager.Middleware(s.handleStackAction("pull")))
	s.mux.Handle("POST /api/stacks/{name}/update", s.authManager.Middleware(s.handleStackAction("update")))
	s.mux.Handle("GET /api/stacks/{name}/logs", s.authManager.Middleware(http.HandlerFunc(s.handleStreamLogs)))

	// Co-located embedded Svelte SPA
	s.mux.Handle("/", ui.Handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("composebox_session")
	authenticated := false
	if err == nil && s.authManager.ValidateToken(cookie.Value) {
		authenticated = true
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"auth_enabled":   s.authManager.IsAuthEnabled(),
		"setup_required": s.authManager.IsSetupRequired(),
		"authenticated":  authenticated,
		"version":        s.version,
	})
}

type authRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	token, err := s.authManager.Setup(req.Password)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.authManager.SetCookie(w, r, token)
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	token, err := s.authManager.Login(req.Password, s.getClientIP(r))
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.authManager.SetCookie(w, r, token)
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("composebox_session"); err == nil && cookie.Value != "" {
		s.authManager.RevokeToken(cookie.Value)
	}
	s.authManager.ClearCookie(w, r)
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleListStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.scanner.Discover()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	for i := range stacks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			containers, status, runningCount, err := s.dockerClient.GetContainers(ctx, stacks[idx].Path, stacks[idx].ComposeFile)
			if err == nil {
				declared := stack.GetDeclaredContainers(filepath.Join(stacks[idx].Path, stacks[idx].ComposeFile))
				stacks[idx].Containers = mergeContainers(containers, declared)
				stacks[idx].Status = status
				if len(stacks[idx].Containers) > stacks[idx].TotalServices {
					stacks[idx].TotalServices = len(stacks[idx].Containers)
				}
				stacks[idx].RunningCount = runningCount
			} else {
				stacks[idx].Status = stack.StatusUnknown
			}
		}(i)
	}
	wg.Wait()

	respondJSON(w, http.StatusOK, stacks)
}

func (s *Server) handleGetStack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	details, err := s.scanner.GetStack(name)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	containers, status, runningCount, err := s.dockerClient.GetContainers(ctx, details.Path, details.ComposeFile)
	if err == nil {
		declared := stack.GetDeclaredContainers(filepath.Join(details.Path, details.ComposeFile))
		details.Containers = mergeContainers(containers, declared)
		details.Status = status
		if len(details.Containers) > details.TotalServices {
			details.TotalServices = len(details.Containers)
		}
		details.RunningCount = runningCount
	}

	respondJSON(w, http.StatusOK, details)
}

func mergeContainers(live []stack.Container, declared []stack.Container) []stack.Container {
	if len(live) == 0 {
		return declared
	}
	if len(declared) == 0 {
		return live
	}

	result := make([]stack.Container, len(live))
	copy(result, live)

	existing := make(map[string]bool)
	for _, c := range live {
		if c.Service != "" {
			existing[c.Service] = true
		}
		existing[c.Name] = true
	}

	for _, d := range declared {
		if !existing[d.Service] && !existing[d.Name] {
			result = append(result, d)
		}
	}

	return result
}

type saveRequest struct {
	ComposeContent string `json:"compose_content"`
	EnvContent     string `json:"env_content"`
}

func (s *Server) handleSaveStack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.scanner.SaveFiles(name, req.ComposeContent, req.EnvContent); err != nil {
		if strings.Contains(err.Error(), "YAML") || strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "invalid") {
			respondError(w, http.StatusBadRequest, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	respondJSON(w, http.StatusOK, stack.ActionResponse{
		Success: true,
		Message: fmt.Sprintf("Stack '%s' configuration saved successfully", name),
	})
}

type createStackRequest struct {
	Name           string `json:"name"`
	ComposeContent string `json:"compose_content"`
	EnvContent     string `json:"env_content"`
}

func (s *Server) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	var req createStackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.scanner.CreateStack(req.Name, req.ComposeContent, req.EnvContent); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, stack.ActionResponse{
		Success: true,
		Message: fmt.Sprintf("Stack '%s' created successfully", req.Name),
	})
}

func (s *Server) handleDeleteStack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	details, err := s.scanner.GetStack(name)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, _, runningCount, err := s.dockerClient.GetContainers(ctx, details.Path, details.ComposeFile)
	if err == nil && runningCount > 0 {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("cannot delete running stack '%s'. Please stop it first.", name))
		return
	}

	if err := s.scanner.DeleteStack(name); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dockerClient.RemoveLock(details.Path)

	respondJSON(w, http.StatusOK, stack.ActionResponse{
		Success: true,
		Message: fmt.Sprintf("Stack '%s' deleted successfully", name),
	})
}

func (s *Server) handleStackAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		details, err := s.scanner.GetStack(name)
		if err != nil {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}

		// Decouple operation execution from HTTP client disconnects so in-flight compose commands
		// run to completion safely and don't leave stacks in a half-created/corrupted state.
		actionTimeout := 5 * time.Minute
		if action == "pull" || action == "update" {
			actionTimeout = 15 * time.Minute
		}
		opCtx, opCancel := context.WithTimeout(context.WithoutCancel(r.Context()), actionTimeout)
		defer opCancel()

		if r.Header.Get("Accept") == "application/json" {
			var output string
			var actionErr error
			switch action {
			case "up":
				output, actionErr = s.dockerClient.Up(opCtx, details.Path, details.ComposeFile)
			case "down":
				output, actionErr = s.dockerClient.Down(opCtx, details.Path, details.ComposeFile)
			case "restart":
				output, actionErr = s.dockerClient.Restart(opCtx, details.Path, details.ComposeFile)
			case "pull":
				output, actionErr = s.dockerClient.Pull(opCtx, details.Path, details.ComposeFile)
			case "update":
				output, actionErr = s.dockerClient.Update(opCtx, details.Path, details.ComposeFile)
			default:
				respondError(w, http.StatusBadRequest, "unknown action")
				return
			}

			if actionErr != nil {
				respondJSON(w, http.StatusInternalServerError, stack.ActionResponse{
					Success: false,
					Message: actionErr.Error(),
					Output:  output,
				})
				return
			}

			respondJSON(w, http.StatusOK, stack.ActionResponse{
				Success: true,
				Message: fmt.Sprintf("Stack '%s' action '%s' completed successfully", name, action),
				Output:  output,
			})
			return
		}

		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		if flusher != nil {
			flusher.Flush()
		}

		fw := &flushWriter{w: w, flusher: flusher}
		fmt.Fprintf(fw, "$ docker compose %s\n\n", action)

		var actionErr error
		switch action {
		case "up":
			actionErr = s.dockerClient.UpStream(opCtx, details.Path, details.ComposeFile, fw)
		case "down":
			actionErr = s.dockerClient.DownStream(opCtx, details.Path, details.ComposeFile, fw)
		case "restart":
			actionErr = s.dockerClient.RestartStream(opCtx, details.Path, details.ComposeFile, fw)
		case "pull":
			actionErr = s.dockerClient.PullStream(opCtx, details.Path, details.ComposeFile, fw)
		case "update":
			actionErr = s.dockerClient.UpdateStream(opCtx, details.Path, details.ComposeFile, fw)
		default:
			fmt.Fprintf(fw, "Error: unknown action '%s'\n", action)
			return
		}

		if actionErr != nil {
			fmt.Fprintf(fw, "\n[ERROR] %v\n", actionErr)
		} else {
			fmt.Fprintf(fw, "\n[SUCCESS] Stack '%s' action '%s' completed successfully.\n", name, action)
		}
	}
}

type flushWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (fw *flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.flusher != nil {
		fw.flusher.Flush()
	}
	return n, err
}

func (s *Server) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	details, err := s.scanner.GetStack(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()

	go func() {
		defer pipeWriter.Close()
		_ = s.dockerClient.StreamLogs(r.Context(), details.Path, details.ComposeFile, pipeWriter)
	}()

	scanner := bufio.NewScanner(pipeReader)
	// Support log lines up to 1MB without terminating the stream
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := stack.CleanLogLine(scanner.Text())
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}
}

func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none';")

		// Strict CSRF and Origin defense on state-changing requests
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			// 1. Verify Origin / Referer
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")

			if origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !isSameHost(parsed.Host, r.Host) {
					respondError(w, http.StatusForbidden, "cross-origin request blocked: origin mismatch")
					return
				}
			} else if referer != "" {
				parsed, err := url.Parse(referer)
				if err != nil || !isSameHost(parsed.Host, r.Host) {
					respondError(w, http.StatusForbidden, "cross-origin request blocked: referer mismatch")
					return
				}
			} else {
				// Both Origin and Referer are absent on a mutation request.
				// Require an explicit custom header or bearer authorization to block blind cross-site submissions.
				if r.Header.Get("X-Composebox-Client") != "true" && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					respondError(w, http.StatusForbidden, "cross-origin request blocked: missing verification headers")
					return
				}
			}

			// 2. Enforce custom header on all mutating API calls except public auth endpoints (which use strict SameSite cookies & body parsing)
			if strings.HasPrefix(r.URL.Path, "/api/stacks") {
				if r.Header.Get("X-Composebox-Client") != "true" && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					respondError(w, http.StatusForbidden, "request blocked: missing X-Composebox-Client header")
					return
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) getClientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	// Only trust forwarding headers if the remote caller is a trusted proxy
	if s.isTrustedProxy(r.RemoteAddr) {
		if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
			return strings.TrimSpace(cf)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}

	return remoteHost
}

func splitHostAndPort(raw string) (string, string) {
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return strings.TrimSpace(raw), ""
	}
	return host, port
}

func isSameHost(originHost, requestHost string) bool {
	h1, p1 := splitHostAndPort(originHost)
	h2, p2 := splitHostAndPort(requestHost)

	if !strings.EqualFold(h1, h2) {
		return false
	}

	// If both specify ports, ports must match
	if p1 != "" && p2 != "" {
		return p1 == p2
	}

	// Standard default ports (80 for HTTP, 443 for HTTPS) match empty port
	if (p1 == "80" || p1 == "443") && p2 == "" {
		return true
	}
	if (p2 == "80" || p2 == "443") && p1 == "" {
		return true
	}

	// If one specifies an explicit custom port and the other does not, reject
	return p1 == p2
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("[HTTP] %s %s %d %s", r.Method, r.URL.Path, rec.status, duration.Truncate(time.Millisecond))
		}
	})
}
