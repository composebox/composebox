package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	validNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
	composeFileNames = []string{
		"compose.yaml",
		"compose.yml",
		"docker-compose.yaml",
		"docker-compose.yml",
	}
)

// Scanner handles discovery and file I/O for compose stacks.
type Scanner struct {
	baseDir string
}

// NewScanner creates a new Scanner for the given base directory.
func NewScanner(baseDir string) *Scanner {
	return &Scanner{baseDir: filepath.Clean(baseDir)}
}

// BaseDir returns the root stacks directory.
func (s *Scanner) BaseDir() string {
	return s.baseDir
}

// Discover scans the base directory and returns all found stacks.
func (s *Scanner) Discover() ([]Stack, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read stacks directory: %w", err)
	}

	stacks := make([]Stack, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		stackPath := filepath.Join(s.baseDir, name)
		composeFile := s.findComposeFile(stackPath)
		if composeFile == "" {
			continue
		}

		hasEnv := fileExists(filepath.Join(stackPath, ".env"))

		stacks = append(stacks, Stack{
			Name:        name,
			Path:        stackPath,
			ComposeFile: composeFile,
			HasEnv:      hasEnv,
			Status:      StatusUnknown,
		})
	}

	return stacks, nil
}

// GetStack reads a single stack's metadata and file contents.
func (s *Scanner) GetStack(name string) (*Details, error) {
	if !IsValidName(name) {
		return nil, fmt.Errorf("invalid stack name '%s': must contain only letters, numbers, dashes, and underscores", name)
	}

	stackPath := filepath.Join(s.baseDir, name)
	composeFileName := s.findComposeFile(stackPath)
	if composeFileName == "" {
		return nil, fmt.Errorf("no compose file found in stack: %s", name)
	}

	composeContent, err := os.ReadFile(filepath.Join(stackPath, composeFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var envContent []byte
	envPath := filepath.Join(stackPath, ".env")
	if fileExists(envPath) {
		envContent, _ = os.ReadFile(envPath)
	}

	return &Details{
		Stack: Stack{
			Name:        name,
			Path:        stackPath,
			ComposeFile: composeFileName,
			HasEnv:      fileExists(envPath),
			Status:      StatusUnknown,
		},
		ComposeContent: string(composeContent),
		EnvContent:     string(envContent),
	}, nil
}

// SaveFiles writes updated compose and .env contents to disk atomically.
func (s *Scanner) SaveFiles(name, composeContent, envContent string) error {
	if !IsValidName(name) {
		return fmt.Errorf("invalid stack name '%s'", name)
	}
	if strings.TrimSpace(composeContent) == "" {
		return fmt.Errorf("compose configuration cannot be empty")
	}

	stackPath := filepath.Join(s.baseDir, name)
	composeFileName := s.findComposeFile(stackPath)
	if composeFileName == "" {
		composeFileName = "compose.yaml"
	}

	if err := os.MkdirAll(stackPath, 0755); err != nil {
		return fmt.Errorf("failed to create stack directory: %w", err)
	}

	// Write compose file atomically
	composeFilePath := filepath.Join(stackPath, composeFileName)
	if err := atomicWriteFile(composeFilePath, []byte(composeContent), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	// Write .env file if provided or already exists
	envPath := filepath.Join(stackPath, ".env")
	if envContent != "" || fileExists(envPath) {
		if err := atomicWriteFile(envPath, []byte(envContent), 0600); err != nil {
			return fmt.Errorf("failed to write .env file: %w", err)
		}
	}

	return nil
}

// CreateStack scaffolds a new stack directory with initial compose and optional .env files.
func (s *Scanner) CreateStack(name, composeContent, envContent string) error {
	trimmed := strings.TrimSpace(name)
	if !IsValidName(trimmed) {
		return fmt.Errorf("invalid stack name '%s': must contain only letters, numbers, dashes, and underscores", trimmed)
	}

	stackPath := filepath.Join(s.baseDir, trimmed)
	if _, err := os.Stat(stackPath); err == nil {
		return fmt.Errorf("stack directory '%s' already exists", trimmed)
	}

	if strings.TrimSpace(composeContent) == "" {
		composeContent = fmt.Sprintf("services:\n  app:\n    image: nginx:alpine\n    container_name: %s\n    restart: unless-stopped\n", trimmed)
	}

	return s.SaveFiles(trimmed, composeContent, envContent)
}

// DeleteStack safely removes a stack directory from disk.
func (s *Scanner) DeleteStack(name string) error {
	trimmed := strings.TrimSpace(name)
	if !IsValidName(trimmed) {
		return fmt.Errorf("invalid stack name '%s'", trimmed)
	}

	stackPath := filepath.Join(s.baseDir, trimmed)
	if _, err := os.Stat(stackPath); os.IsNotExist(err) {
		return fmt.Errorf("stack directory '%s' does not exist", trimmed)
	}

	return os.RemoveAll(stackPath)
}

// IsValidName checks if a stack name meets strict filesystem and container naming rules.
func IsValidName(name string) bool {
	return validNameRegex.MatchString(name) && len(name) <= 64
}

func (s *Scanner) findComposeFile(dir string) string {
	for _, filename := range composeFileNames {
		if fileExists(filepath.Join(dir, filename)) {
			return filename
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmpFile, err := os.CreateTemp(dir, "tmp-*")
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
