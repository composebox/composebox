package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName        = "composebox_session"
	tokenTTL          = 7 * 24 * time.Hour // 7-day session duration
	authFileName      = ".composebox.auth"
	secretFileName    = ".composebox.secret"
	maxFailedAttempts = 5
	lockoutDuration   = 30 * time.Second
)

type storedAuth struct {
	PasswordHash string `json:"password_hash"`
}

// Manager handles user authentication, session tokens, rate limiting, and middleware.
type Manager struct {
	mu             sync.Mutex
	authFilePath   string
	secretFilePath string
	envPassword    string
	noAuth         bool
	sessionSecret  []byte
	failedAttempts int
	lockoutUntil   time.Time
}

// New creates an Auth Manager and ensures the stacks directory and session secret exist.
func New(stacksDir string, envPassword string, noAuth bool) (*Manager, error) {
	cleanDir := filepath.Clean(stacksDir)
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to initialize stacks directory: %w", err)
	}

	m := &Manager{
		authFilePath:   filepath.Join(cleanDir, authFileName),
		secretFilePath: filepath.Join(cleanDir, secretFileName),
		envPassword:    envPassword,
		noAuth:         noAuth,
	}

	if noAuth {
		return m, nil
	}

	// Persistent secret key management so sessions survive restarts
	if fileExists(m.secretFilePath) {
		data, err := os.ReadFile(m.secretFilePath)
		if err == nil {
			sec, err := hex.DecodeString(strings.TrimSpace(string(data)))
			if err == nil && len(sec) >= 32 {
				m.sessionSecret = sec
			}
		}
	}

	if len(m.sessionSecret) == 0 {
		m.sessionSecret = make([]byte, 32)
		if _, err := rand.Read(m.sessionSecret); err != nil {
			return nil, fmt.Errorf("failed to generate random secret: %w", err)
		}
		if err := atomicWriteFile(m.secretFilePath, []byte(hex.EncodeToString(m.sessionSecret)), 0600); err != nil {
			return nil, fmt.Errorf("failed to persist session secret: %w", err)
		}
	}

	return m, nil
}

// IsAuthEnabled returns true if authentication is active.
func (m *Manager) IsAuthEnabled() bool {
	return !m.noAuth
}

// IsSetupRequired returns true if no password has been configured yet.
func (m *Manager) IsSetupRequired() bool {
	if m.noAuth || m.envPassword != "" {
		return false
	}
	return !fileExists(m.authFilePath)
}

// Setup creates the initial admin password atomically and prevents race conditions.
func (m *Manager) Setup(password string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.noAuth {
		return "", errors.New("auth is disabled")
	}
	if !m.IsSetupRequired() {
		return "", errors.New("admin password is already set")
	}
	if len(password) < 6 {
		return "", errors.New("password must be at least 6 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	sa := storedAuth{PasswordHash: string(hash)}
	data, err := json.Marshal(sa)
	if err != nil {
		return "", err
	}

	if err := atomicWriteFile(m.authFilePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write auth file: %w", err)
	}

	m.failedAttempts = 0
	return m.generateToken(), nil
}

// Login verifies the password with brute-force protection and returns a new session token.
func (m *Manager) Login(password string) (string, error) {
	if m.noAuth {
		return "no-auth-token", nil
	}

	m.mu.Lock()
	if time.Now().Before(m.lockoutUntil) {
		remaining := int(time.Until(m.lockoutUntil).Seconds()) + 1
		m.mu.Unlock()
		return "", fmt.Errorf("too many failed attempts, try again in %ds", remaining)
	}
	m.mu.Unlock()

	// 1. Environment variable override
	if m.envPassword != "" {
		if subtle.ConstantTimeCompare([]byte(password), []byte(m.envPassword)) == 1 {
			m.resetFailures()
			return m.generateToken(), nil
		}
		m.recordFailure()
		return "", errors.New("invalid password")
	}

	// 2. Auth file check
	data, err := os.ReadFile(m.authFilePath)
	if err != nil {
		return "", errors.New("no password configured, setup required")
	}

	var sa storedAuth
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", errors.New("corrupted auth file")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(sa.PasswordHash), []byte(password)); err != nil {
		m.recordFailure()
		return "", errors.New("invalid password")
	}

	m.resetFailures()
	return m.generateToken(), nil
}

func (m *Manager) recordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedAttempts++
	if m.failedAttempts >= maxFailedAttempts {
		m.lockoutUntil = time.Now().Add(lockoutDuration)
		m.failedAttempts = 0
	}
}

func (m *Manager) resetFailures() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedAttempts = 0
	m.lockoutUntil = time.Time{}
}

// ValidateToken verifies a token signature and expiration in constant time.
func (m *Manager) ValidateToken(token string) bool {
	if m.noAuth {
		return true
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	expStr, sigHex := parts[0], parts[1]
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > expUnix {
		return false
	}

	expectedSig := m.sign(expStr)
	return hmac.Equal([]byte(sigHex), []byte(expectedSig))
}

// SetCookie attaches the session cookie with dynamic Secure detection.
func (m *Manager) SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(tokenTTL),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie invalidates the session cookie.
func (m *Manager) ClearCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Middleware protects endpoints requiring authentication.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.noAuth {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Authorization: Bearer token
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if m.ValidateToken(token) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// 2. Cookie
		cookie, err := r.Cookie(cookieName)
		if err == nil && m.ValidateToken(cookie.Value) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
	})
}

func (m *Manager) generateToken() string {
	expUnix := time.Now().Add(tokenTTL).Unix()
	expStr := strconv.FormatInt(expUnix, 10)
	sig := m.sign(expStr)
	return fmt.Sprintf("%s.%s", expStr, sig)
}

func (m *Manager) sign(data string) string {
	h := hmac.New(sha256.New, m.sessionSecret)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmpFile, err := os.CreateTemp(dir, "tmp-auth-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	return os.Rename(tmpName, filename)
}
