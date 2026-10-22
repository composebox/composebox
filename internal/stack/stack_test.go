package stack

import (
	"os"
	"testing"
)

func TestScanner(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "composebox-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	scanner := NewScanner(tempDir)

	// Test 1: Empty discovery
	stacks, err := scanner.Discover()
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks, got %d", len(stacks))
	}

	// Test 2: Create a stack
	composeYaml := "services:\n  web:\n    image: nginx\n"
	envData := "FOO=BAR\n"
	if err := scanner.CreateStack("test-app", composeYaml, envData); err != nil {
		t.Fatalf("failed to create stack: %v", err)
	}

	// Test 3: Discover created stack
	stacks, err = scanner.Discover()
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(stacks))
	}
	if stacks[0].Name != "test-app" {
		t.Fatalf("expected name 'test-app', got '%s'", stacks[0].Name)
	}
	if !stacks[0].HasEnv {
		t.Fatalf("expected HasEnv to be true")
	}

	// Test 4: Get stack details
	details, err := scanner.GetStack("test-app")
	if err != nil {
		t.Fatalf("get stack failed: %v", err)
	}
	if details.ComposeContent != composeYaml {
		t.Fatalf("compose content mismatch")
	}
	if details.EnvContent != envData {
		t.Fatalf("env content mismatch")
	}

	// Test 5: Atomic update
	newCompose := "services:\n  web:\n    image: redis\n"
	if err := scanner.SaveFiles("test-app", newCompose, envData); err != nil {
		t.Fatalf("failed to save files: %v", err)
	}
	details, err = scanner.GetStack("test-app")
	if err != nil {
		t.Fatalf("get stack failed: %v", err)
	}
	if details.ComposeContent != newCompose {
		t.Fatalf("expected updated compose content")
	}

	// Test 6: Delete stack
	if err := scanner.DeleteStack("test-app"); err != nil {
		t.Fatalf("failed to delete stack: %v", err)
	}
	stacks, err = scanner.Discover()
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks after delete, got %d", len(stacks))
	}
}

func TestValidation(t *testing.T) {
	valid := []string{"web", "my-app", "app_1", "STACK"}
	invalid := []string{"", "../bad", "foo/bar", "with spaces", "semi;colon"}

	for _, name := range valid {
		if !IsValidName(name) {
			t.Errorf("expected '%s' to be valid", name)
		}
	}

	for _, name := range invalid {
		if IsValidName(name) {
			t.Errorf("expected '%s' to be invalid", name)
		}
	}
}

func TestParseContainersJSON(t *testing.T) {
	// 1. NDJSON format (Docker Compose v2.21+) with mixed running and exited states
	ndjson := `{"Name":"app-web-1","Service":"web","Image":"nginx:alpine","State":"running","Status":"Up 2 hours","Publishers":[{"TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"}]}
{"Name":"app-db-1","Service":"db","Image":"postgres:16","State":"exited","Status":"Exited (1) 5 minutes ago"}`

	containers, status, running := parseContainersJSON(ndjson)
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers from NDJSON, got %d", len(containers))
	}
	if status != StatusPartial {
		t.Errorf("expected StatusPartial, got %s", status)
	}
	if running != 1 {
		t.Errorf("expected running count 1, got %d", running)
	}
	if containers[0].Ports != "8080:80" {
		t.Errorf("expected port '8080:80', got '%s'", containers[0].Ports)
	}

	// 2. JSON Array format (older Docker Compose v2)
	jsonArray := `[{"Name":"app-redis-1","Service":"redis","Image":"redis:alpine","State":"Exited","Status":"Exited (0)"}]`
	containersArr, statusArr, runningArr := parseContainersJSON(jsonArray)
	if len(containersArr) != 1 {
		t.Fatalf("expected 1 container from JSON array, got %d", len(containersArr))
	}
	if statusArr != StatusExited {
		t.Errorf("expected StatusExited, got %s", statusArr)
	}
	if runningArr != 0 {
		t.Errorf("expected running count 0, got %d", runningArr)
	}
}

func TestCleanLogLine(t *testing.T) {
	raw := "opengist  | \x1b[90m12:30:47\x1b[0m \x1b[32mINF\x1b[0m \x1b[38;5;208mAUTH\x1b[0m ✓ Ready\x07\xff"
	got := CleanLogLine(raw)
	want := "opengist  | 12:30:47 INF AUTH ✓ Ready"
	if got != want {
		t.Errorf("CleanLogLine() = %q, want %q", got, want)
	}
}
