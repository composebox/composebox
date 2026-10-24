package stack

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

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

// Stop runs docker compose stop with per-stack serialization (halts containers without removing them).
func (c *DockerClient) Stop(ctx context.Context, stackPath, composeFile string) (string, error) {
	var buf bytes.Buffer
	err := c.StopStream(ctx, stackPath, composeFile, &buf)
	return strings.TrimSpace(buf.String()), err
}

// StopStream executes docker compose stop and streams output directly to out.
func (c *DockerClient) StopStream(ctx context.Context, stackPath, composeFile string, out io.Writer) error {
	lock := c.getStackLock(stackPath)
	lock.Lock()
	defer lock.Unlock()

	return c.runCommandStream(ctx, stackPath, composeFile, out, "stop")
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	cmd.Stdout = w
	cmd.Stderr = w

	return cmd.Run()
}

func (c *DockerClient) runCommandStream(ctx context.Context, stackPath, composeFile string, out io.Writer, args ...string) error {
	fullComposePath := filepath.Join(stackPath, composeFile)
	baseArgs := []string{"compose", "-f", fullComposePath}
	fullArgs := append(baseArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = stackPath
	cmd.WaitDelay = 3 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command 'docker %s' failed: %w", strings.Join(fullArgs, " "), err)
	}
	return nil
}

// DiskCategory represents storage usage of a specific Docker subsystem.
type DiskCategory struct {
	Type        string `json:"type"`
	TotalCount  int    `json:"total_count"`
	ActiveCount int    `json:"active_count"`
	Size        string `json:"size"`
	Reclaimable string `json:"reclaimable"`
}

// DetailedImage represents an individual Docker image item.
type DetailedImage struct {
	ID           string `json:"id"`
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
	Size         string `json:"size"`
	UniqueSize   string `json:"unique_size"`
	CreatedSince string `json:"created_since"`
	Containers   string `json:"containers"`
	IsDangling   bool   `json:"is_dangling"`
}

// DetailedContainer represents an individual Docker container.
type DetailedContainer struct {
	ID         string `json:"id"`
	Names      string `json:"names"`
	Image      string `json:"image"`
	Status     string `json:"status"`
	Size       string `json:"size"`
	RunningFor string `json:"running_for"`
}

// DetailedVolume represents an individual Docker volume.
type DetailedVolume struct {
	Name  string `json:"name"`
	Size  string `json:"size"`
	Links string `json:"links"`
}

// DetailedBuildCache represents an individual Docker build cache entry.
type DetailedBuildCache struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Size        string `json:"size"`
	InUse       string `json:"in_use"`
	CacheType   string `json:"cache_type"`
}

// DiskUsageReport summarizes Docker system storage allocation and item breakdowns.
type DiskUsageReport struct {
	Categories       []DiskCategory       `json:"categories"`
	TotalSize        string               `json:"total_size"`
	TotalReclaimable string               `json:"total_reclaimable"`
	Images           []DetailedImage      `json:"images"`
	Containers       []DetailedContainer  `json:"containers"`
	Volumes          []DetailedVolume     `json:"volumes"`
	BuildCache       []DetailedBuildCache `json:"build_cache"`
}

type rawDiskItem struct {
	Type        string `json:"Type"`
	TotalCount  string `json:"TotalCount"`
	Active      string `json:"Active"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

type rawDfVerbose struct {
	Images []struct {
		ID           string `json:"ID"`
		Repository   string `json:"Repository"`
		Tag          string `json:"Tag"`
		Size         string `json:"Size"`
		UniqueSize   string `json:"UniqueSize"`
		CreatedSince string `json:"CreatedSince"`
		Containers   string `json:"Containers"`
	} `json:"Images"`
	Containers []struct {
		ID         string `json:"ID"`
		Names      string `json:"Names"`
		Image      string `json:"Image"`
		Status     string `json:"Status"`
		Size       string `json:"Size"`
		RunningFor string `json:"RunningFor"`
	} `json:"Containers"`
	Volumes []struct {
		Name  string `json:"Name"`
		Size  string `json:"Size"`
		Links string `json:"Links"`
	} `json:"Volumes"`
	BuildCache []struct {
		ID          string `json:"ID"`
		Description string `json:"Description"`
		Size        string `json:"Size"`
		InUse       string `json:"InUse"`
		CacheType   string `json:"CacheType"`
	} `json:"BuildCache"`
}

// GetSystemDiskUsage executes 'docker system df' and parses the output.
func (c *DockerClient) GetSystemDiskUsage(ctx context.Context) (*DiskUsageReport, error) {
	cmd := exec.CommandContext(ctx, "docker", "system", "df", "--format", "{{json .}}")
	cmd.WaitDelay = 3 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query docker system df: %w", err)
	}

	report := &DiskUsageReport{
		Categories: make([]DiskCategory, 0),
		Images:     make([]DetailedImage, 0),
		Containers: make([]DetailedContainer, 0),
		Volumes:    make([]DetailedVolume, 0),
		BuildCache: make([]DetailedBuildCache, 0),
	}

	var totalSizeBytes int64
	var totalReclaimBytes int64

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw rawDiskItem
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		totalCount, _ := strconv.Atoi(raw.TotalCount)
		activeCount, _ := strconv.Atoi(raw.Active)

		category := DiskCategory{
			Type:        raw.Type,
			TotalCount:  totalCount,
			ActiveCount: activeCount,
			Size:        raw.Size,
			Reclaimable: raw.Reclaimable,
		}
		report.Categories = append(report.Categories, category)

		totalSizeBytes += parseDockerSize(raw.Size)
		totalReclaimBytes += parseDockerSize(raw.Reclaimable)
	}

	report.TotalSize = formatDockerSize(totalSizeBytes)
	report.TotalReclaimable = formatDockerSize(totalReclaimBytes)

	// Fetch detailed items via 'docker system df -v'
	cmdV := exec.CommandContext(ctx, "docker", "system", "df", "-v", "--format", "{{json .}}")
	cmdV.WaitDelay = 4 * time.Second
	cmdV.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmdV.Cancel = func() error {
		if cmdV.Process != nil {
			return syscall.Kill(-cmdV.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	if outV, err := cmdV.Output(); err == nil {
		var vData rawDfVerbose
		if err := json.Unmarshal(outV, &vData); err == nil {
			for _, img := range vData.Images {
				shortID := strings.TrimPrefix(img.ID, "sha256:")
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				isDangling := img.Repository == "<none>" || img.Tag == "<none>"
				report.Images = append(report.Images, DetailedImage{
					ID:           shortID,
					Repository:   img.Repository,
					Tag:          img.Tag,
					Size:         img.Size,
					UniqueSize:   img.UniqueSize,
					CreatedSince: img.CreatedSince,
					Containers:   img.Containers,
					IsDangling:   isDangling,
				})
			}

			for _, ctr := range vData.Containers {
				shortID := ctr.ID
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				report.Containers = append(report.Containers, DetailedContainer{
					ID:         shortID,
					Names:      ctr.Names,
					Image:      ctr.Image,
					Status:     ctr.Status,
					Size:       ctr.Size,
					RunningFor: ctr.RunningFor,
				})
			}

			for _, vol := range vData.Volumes {
				report.Volumes = append(report.Volumes, DetailedVolume{
					Name:  vol.Name,
					Size:  vol.Size,
					Links: vol.Links,
				})
			}

			for _, bc := range vData.BuildCache {
				shortID := bc.ID
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				report.BuildCache = append(report.BuildCache, DetailedBuildCache{
					ID:          shortID,
					Description: bc.Description,
					Size:        bc.Size,
					InUse:       bc.InUse,
					CacheType:   bc.CacheType,
				})
			}
		}
	}

	return report, nil
}

// PruneSystem safely executes selective prune operations.
func (c *DockerClient) PruneSystem(ctx context.Context, target string, out io.Writer) error {
	switch target {
	case "images":
		return c.runRawCommand(ctx, out, "image", "prune", "-f")
	case "build_cache":
		return c.runRawCommand(ctx, out, "builder", "prune", "-f")
	case "containers":
		return c.runRawCommand(ctx, out, "container", "prune", "-f")
	case "all_safe":
		fmt.Fprintln(out, "--> Pruning dangling images...")
		if err := c.runRawCommand(ctx, out, "image", "prune", "-f"); err != nil {
			return err
		}
		fmt.Fprintln(out, "\n--> Pruning build cache...")
		if err := c.runRawCommand(ctx, out, "builder", "prune", "-f"); err != nil {
			return err
		}
		fmt.Fprintln(out, "\n--> Pruning stopped containers...")
		return c.runRawCommand(ctx, out, "container", "prune", "-f")
	default:
		return fmt.Errorf("invalid prune target '%s': allowed targets are 'images', 'build_cache', 'containers', 'all_safe'", target)
	}
}

// PruneItem deletes a specific Docker item (image, container, volume, build_cache).
func (c *DockerClient) PruneItem(ctx context.Context, itemType, identifier string, out io.Writer) error {
	switch itemType {
	case "image":
		return c.runRawCommand(ctx, out, "image", "rm", identifier)
	case "container":
		return c.runRawCommand(ctx, out, "container", "rm", identifier)
	case "volume":
		return c.runRawCommand(ctx, out, "volume", "rm", identifier)
	case "build_cache":
		return c.runRawCommand(ctx, out, "builder", "prune", "--filter", "id="+identifier, "-f")
	default:
		return fmt.Errorf("unknown item type '%s'", itemType)
	}
}

func (c *DockerClient) runRawCommand(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = 3 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command 'docker %s' failed: %w", strings.Join(args, " "), err)
	}
	return nil
}

func parseDockerSize(s string) int64 {
	// Reclaimable might look like "9.701GB (41%)"
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0
	}
	raw := parts[0]
	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(raw, "GB") {
		multiplier = 1000 * 1000 * 1000
		numStr = strings.TrimSuffix(raw, "GB")
	} else if strings.HasSuffix(raw, "MB") {
		multiplier = 1000 * 1000
		numStr = strings.TrimSuffix(raw, "MB")
	} else if strings.HasSuffix(raw, "kB") || strings.HasSuffix(raw, "KB") {
		multiplier = 1000
		numStr = strings.TrimSuffix(strings.TrimSuffix(raw, "kB"), "KB")
	} else if strings.HasSuffix(raw, "B") {
		numStr = strings.TrimSuffix(raw, "B")
	} else {
		return 0
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

func formatDockerSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const (
		kB = 1000
		MB = 1000 * kB
		GB = 1000 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= kB:
		return fmt.Sprintf("%.1f kB", float64(bytes)/float64(kB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
