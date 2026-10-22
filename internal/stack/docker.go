package stack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ansiEscapeRegex = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\a\x1b]*(?:\a|\x1b\\)|[@-Z\\-_])`)

// CleanLogLine strips ANSI escape sequences, invalid UTF-8 bytes, and non-printable control characters.
func CleanLogLine(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = ansiEscapeRegex.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r >= 0x20 && r != 0x7f {
			return r
		}
		return -1
	}, s)
}

// DockerClient handles executing docker compose CLI commands with per-stack locking.
type DockerClient struct {
	locks sync.Map // map[string]*sync.Mutex
}

// NewDockerClient creates a new docker compose client.
func NewDockerClient() *DockerClient {
	return &DockerClient{}
}

func (c *DockerClient) getStackLock(stackPath string) *sync.Mutex {
	val, _ := c.locks.LoadOrStore(stackPath, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// RemoveLock cleans up the mutex entry when a stack is deleted.
func (c *DockerClient) RemoveLock(stackPath string) {
	c.locks.Delete(stackPath)
}

type rawContainer struct {
	Name       string `json:"Name"`
	Names      string `json:"Names"`
	Service    string `json:"Service"`
	Image      string `json:"Image"`
	State      string `json:"State"`
	Status     string `json:"Status"`
	Health     string `json:"Health"`
	Ports      string `json:"Ports"`
	Publishers []struct {
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

// GetContainers returns the live container list (including exited containers), aggregate status, and running count.
func (c *DockerClient) GetContainers(ctx context.Context, stackPath, composeFile string) ([]Container, Status, int, error) {
	fullComposePath := filepath.Join(stackPath, composeFile)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", fullComposePath, "ps", "-a", "--format", "json")
	cmd.Dir = stackPath
	cmd.WaitDelay = 3 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return make([]Container, 0), StatusUnknown, 0, fmt.Errorf("docker compose ps failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	containers, status, runningCount := parseContainersJSON(stdout.String())
	return containers, status, runningCount, nil
}

// parseContainersJSON supports both Docker Compose JSON array output ([{...}]) and NDJSON output ({...}\n{...}).
func parseContainersJSON(rawOutput string) ([]Container, Status, int) {
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" || trimmed == "[]" {
		return make([]Container, 0), StatusStopped, 0
	}

	var raws []rawContainer
	if strings.HasPrefix(trimmed, "[") {
		_ = json.Unmarshal([]byte(trimmed), &raws)
	} else {
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		for {
			var raw rawContainer
			if err := decoder.Decode(&raw); err != nil {
				break
			}
			raws = append(raws, raw)
		}
	}

	containers := make([]Container, 0, len(raws))
	for _, raw := range raws {
		name := raw.Name
		if name == "" {
			name = raw.Names
		}

		var portList []string
		for _, p := range raw.Publishers {
			if p.PublishedPort > 0 {
				portList = append(portList, fmt.Sprintf("%d:%d", p.PublishedPort, p.TargetPort))
			}
		}
		ports := raw.Ports
		if len(portList) > 0 {
			ports = strings.Join(portList, ", ")
		}

		containers = append(containers, Container{
			Name:    name,
			Service: raw.Service,
			Image:   raw.Image,
			State:   raw.State,
			Status:  raw.Status,
			Health:  raw.Health,
			Ports:   ports,
		})
	}

	if len(containers) == 0 {
		return containers, StatusStopped, 0
	}

	runningCount := 0
	for _, ct := range containers {
		if strings.EqualFold(ct.State, "running") {
			runningCount++
		}
	}

	var aggregate Status
	if runningCount == len(containers) {
		aggregate = StatusRunning
	} else if runningCount > 0 {
		aggregate = StatusPartial
	} else {
		aggregate = StatusExited
	}

	return containers, aggregate, runningCount
}

// Up runs docker compose up -d with per-stack serialization.
func (c *DockerClient) Up(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.UpStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// UpStream executes docker compose up -d and streams output directly to out.
func (c *DockerClient) UpStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	return c.runCommandStream(ctx, stackPath, composeFile, out, "up", "-d", "--remove-orphans")
}

// Down runs docker compose down with per-stack serialization.
func (c *DockerClient) Down(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.DownStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// DownStream executes docker compose down and streams output directly to out.
func (c *DockerClient) DownStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	return c.runCommandStream(ctx, stackPath, composeFile, out, "down")
}

// Restart runs docker compose restart with per-stack serialization.
func (c *DockerClient) Restart(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.RestartStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// RestartStream executes docker compose restart and streams output directly to out.
func (c *DockerClient) RestartStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	return c.runCommandStream(ctx, stackPath, composeFile, out, "restart")
}

// Pull runs docker compose pull with per-stack serialization.
func (c *DockerClient) Pull(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.PullStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// PullStream executes docker compose pull and streams output directly to out.
func (c *DockerClient) PullStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	return c.runCommandStream(ctx, stackPath, composeFile, out, "pull")
}

// Update runs docker compose pull followed by docker compose up -d with per-stack serialization.
func (c *DockerClient) Update(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.UpdateStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// UpdateStream runs docker compose pull followed by docker compose up -d streaming output.
func (c *DockerClient) UpdateStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	fmt.Fprintf(out, "--> Pulling latest images from registry...\n")
	if err := c.runCommandStream(ctx, stackPath, composeFile, out, "pull"); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	fmt.Fprintf(out, "\n--> Recreating and applying container changes...\n")
	if err := c.runCommandStream(ctx, stackPath, composeFile, out, "up", "-d", "--remove-orphans"); err != nil {
		return fmt.Errorf("up failed: %w", err)
	}

	return nil
}

// StreamLogs executes docker compose logs -f and writes to w.
func (c *DockerClient) StreamLogs(ctx context.Context, stackPath, composeFile string, w io.Writer) error {
	fullComposePath := filepath.Join(stackPath, composeFile)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", fullComposePath, "logs", "-f", "--tail=100", "--no-color")
	cmd.Dir = stackPath
	cmd.WaitDelay = 3 * time.Second

	cmd.Stdout = w
	cmd.Stderr = w

	return cmd.Run()
}

func (c *DockerClient) runCommand(ctx context.Context, stackPath, composeFile string, args ...string) (string, error) {
	var out bytes.Buffer
	err := c.runCommandStream(ctx, stackPath, composeFile, &out, args...)
	return strings.TrimSpace(out.String()), err
}

func (c *DockerClient) runCommandStream(ctx context.Context, stackPath, composeFile string, out io.Writer, args ...string) error {
	fullComposePath := filepath.Join(stackPath, composeFile)
	baseArgs := []string{"compose", "-f", fullComposePath}
	fullArgs := append(baseArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = stackPath
	cmd.WaitDelay = 3 * time.Second

	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command 'docker %s' failed: %w", strings.Join(fullArgs, " "), err)
	}
	return nil
}
