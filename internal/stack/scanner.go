package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
		declaredCount := CountServices(filepath.Join(stackPath, composeFile))

		stacks = append(stacks, Stack{
			Name:          name,
			Path:          stackPath,
			ComposeFile:   composeFile,
			HasEnv:        hasEnv,
			Status:        StatusUnknown,
			TotalServices: declaredCount,
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

	declaredCount := CountServices(filepath.Join(stackPath, composeFileName))
	warnings, _ := AuditComposeSecurity(string(composeContent))

	return &Details{
		Stack: Stack{
			Name:          name,
			Path:          stackPath,
			ComposeFile:   composeFileName,
			HasEnv:        fileExists(envPath),
			Status:        StatusUnknown,
			TotalServices: declaredCount,
		},
		ComposeContent:   string(composeContent),
		EnvContent:       string(envContent),
		SecurityWarnings: warnings,
	}, nil
}

// AuditComposeSecurity parses compose content, validates YAML syntax, and detects dangerous host access.
func AuditComposeSecurity(composeContent string) ([]string, error) {
	trimmed := strings.TrimSpace(composeContent)
	if trimmed == "" {
		return nil, fmt.Errorf("compose configuration cannot be empty")
	}

	var raw struct {
		Services map[string]struct {
			Privileged  any `yaml:"privileged"`
			Pid         any `yaml:"pid"`
			NetworkMode any `yaml:"network_mode"`
			Volumes     any `yaml:"volumes"`
			CapAdd      any `yaml:"cap_add"`
			SecurityOpt any `yaml:"security_opt"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML syntax: %w", err)
	}

	var warnings []string
	for svcName, svc := range raw.Services {
		if svc.Privileged == true || fmt.Sprint(svc.Privileged) == "true" {
			warnings = append(warnings, fmt.Sprintf("Service '%s' has privileged: true (unrestricted host kernel access)", svcName))
		}
		if fmt.Sprint(svc.Pid) == "host" {
			warnings = append(warnings, fmt.Sprintf("Service '%s' uses host PID namespace (pid: host)", svcName))
		}
		if fmt.Sprint(svc.NetworkMode) == "host" {
			warnings = append(warnings, fmt.Sprintf("Service '%s' uses host networking (network_mode: host)", svcName))
		}

		// Inspect cap_add
		if svc.CapAdd != nil {
			var capList []string
			switch c := svc.CapAdd.(type) {
			case []any:
				for _, item := range c {
					capList = append(capList, strings.ToUpper(fmt.Sprint(item)))
				}
			case string:
				capList = append(capList, strings.ToUpper(c))
			}
			for _, capItem := range capList {
				if capItem == "ALL" || capItem == "SYS_ADMIN" || capItem == "CAP_SYS_ADMIN" {
					warnings = append(warnings, fmt.Sprintf("Service '%s' adds elevated capability (%s)", svcName, capItem))
				}
			}
		}

		// Inspect security_opt
		if svc.SecurityOpt != nil {
			var optList []string
			switch o := svc.SecurityOpt.(type) {
			case []any:
				for _, item := range o {
					optList = append(optList, strings.ToLower(fmt.Sprint(item)))
				}
			case string:
				optList = append(optList, strings.ToLower(o))
			}
			for _, optItem := range optList {
				if strings.Contains(optItem, "unconfined") {
					warnings = append(warnings, fmt.Sprintf("Service '%s' disables container security sandbox (%s)", svcName, optItem))
				}
			}
		}

		// Inspect volume host paths for sensitive host exposure (short and long form)
		if vList, ok := svc.Volumes.([]any); ok {
			for _, v := range vList {
				hostPart := extractVolumeHostPath(v)
				if hostPart == "" {
					continue
				}
				if hostPart == "/" || hostPart == "/etc" || hostPart == "/proc" || hostPart == "/sys" || hostPart == "/root" || hostPart == "/var/run" {
					warnings = append(warnings, fmt.Sprintf("Service '%s' mounts critical host path (%s)", svcName, hostPart))
				}
			}
		}
	}

	return warnings, nil
}

func extractVolumeHostPath(v any) string {
	switch val := v.(type) {
	case string:
		parts := strings.Split(val, ":")
		return strings.TrimSpace(parts[0])
	case map[string]any:
		if src, ok := val["source"]; ok && src != nil {
			return strings.TrimSpace(fmt.Sprint(src))
		}
	case map[any]any:
		if src, ok := val["source"]; ok && src != nil {
			return strings.TrimSpace(fmt.Sprint(src))
		}
	}
	return ""
}

// CountServices parses a compose file and returns the number of declared services.
func CountServices(composeFilePath string) int {
	data, err := os.ReadFile(composeFilePath)
	if err != nil {
		return 0
	}
	var cfg struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return 0
	}
	return len(cfg.Services)
}

// ServiceConfig represents basic container metadata parsed from a compose file.
type ServiceConfig struct {
	ContainerName string `yaml:"container_name"`
	Image         string `yaml:"image"`
	Ports         []any  `yaml:"ports"`
}

// GetDeclaredContainers parses a compose file and returns stub Container structs for defined services.
func GetDeclaredContainers(composeFilePath string) []Container {
	data, err := os.ReadFile(composeFilePath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Services map[string]ServiceConfig `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	containers := make([]Container, 0, len(cfg.Services))
	for svcName, svc := range cfg.Services {
		name := svc.ContainerName
		if name == "" {
			name = svcName
		}
		var ports []string
		for _, p := range svc.Ports {
			switch v := p.(type) {
			case string:
				ports = append(ports, v)
			case int, int64:
				ports = append(ports, fmt.Sprintf("%d", v))
			case map[string]any:
				target := v["target"]
				pub := v["published"]
				if pub != nil && target != nil {
					ports = append(ports, fmt.Sprintf("%v:%v", pub, target))
				} else if target != nil {
					ports = append(ports, fmt.Sprintf("%v", target))
				}
			}
		}
		containers = append(containers, Container{
			Name:    name,
			Service: svcName,
			Image:   svc.Image,
			Status:  "Stopped",
			State:   "stopped",
			Ports:   strings.Join(ports, ", "),
		})
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Service < containers[j].Service
	})

	return containers
}

// SaveFiles writes updated compose and .env contents to disk atomically with YAML syntax validation.
func (s *Scanner) SaveFiles(name, composeContent, envContent string) error {
	if !IsValidName(name) {
		return fmt.Errorf("invalid stack name '%s'", name)
	}
	if strings.TrimSpace(composeContent) == "" {
		return fmt.Errorf("compose configuration cannot be empty")
	}

	// Validate compose YAML syntax before committing to disk
	if _, err := AuditComposeSecurity(composeContent); err != nil {
		return err
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

var allowedDeleteFiles = map[string]bool{
	"compose.yaml":        true,
	"compose.yml":         true,
	"docker-compose.yaml": true,
	"docker-compose.yml":  true,
	".env":                true,
	".env.example":        true,
	"README.md":           true,
	"readme.md":           true,
	".gitignore":          true,
}

// DeleteStack safely removes a stack directory from disk.
// If the directory contains persistent data volumes or subdirectories, deletion is rejected.
// Safe stacks are moved to .trash/ rather than permanently deleted.
func (s *Scanner) DeleteStack(name string) error {
	trimmed := strings.TrimSpace(name)
	if !IsValidName(trimmed) {
		return fmt.Errorf("invalid stack name '%s'", trimmed)
	}

	stackPath := filepath.Join(s.baseDir, trimmed)
	if _, err := os.Stat(stackPath); os.IsNotExist(err) {
		return fmt.Errorf("stack directory '%s' does not exist", trimmed)
	}

	// Safety check: detect non-compose files or data directories
	entries, err := os.ReadDir(stackPath)
	if err != nil {
		return fmt.Errorf("failed to inspect stack directory: %w", err)
	}

	var nonComposeEntries []string
	for _, entry := range entries {
		if entry.IsDir() || !allowedDeleteFiles[entry.Name()] {
			nonComposeEntries = append(nonComposeEntries, entry.Name())
		}
	}

	if len(nonComposeEntries) > 0 {
		return fmt.Errorf("stack directory '%s' contains persistent data or extra directories (%s); remove volumes manually on host before deleting stack to prevent data loss", trimmed, strings.Join(nonComposeEntries, ", "))
	}

	// Move to .trash archive instead of permanent deletion
	trashDir := filepath.Join(s.baseDir, ".trash")
	if err := os.MkdirAll(trashDir, 0700); err != nil {
		return fmt.Errorf("failed to create trash directory: %w", err)
	}

	trashDest := filepath.Join(trashDir, fmt.Sprintf("%s-%d", trimmed, time.Now().UnixNano()))
	if err := os.Rename(stackPath, trashDest); err != nil {
		return fmt.Errorf("failed to move stack to trash: %w", err)
	}

	return nil
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
