package stack

// Status represents the aggregate state of a Compose stack.
type Status string

const (
	StatusRunning Status = "running"
	StatusPartial Status = "partial"
	StatusExited  Status = "exited"
	StatusStopped Status = "stopped"
	StatusUnknown Status = "unknown"
)

// Container represents an individual container within a stack.
type Container struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Health  string `json:"health,omitempty"`
	Ports   string `json:"ports,omitempty"`
}

// Stack represents a discovered compose stack directory.
type Stack struct {
	Name          string      `json:"name"`
	Path          string      `json:"path"`
	ComposeFile   string      `json:"compose_file"`
	HasEnv        bool        `json:"has_env"`
	Status        Status      `json:"status"`
	TotalServices int         `json:"total_services"`
	RunningCount  int         `json:"running_count"`
	Containers    []Container `json:"containers,omitempty"`
}

// Details represents full file contents and metadata for a single stack.
type Details struct {
	Stack
	ComposeContent string `json:"compose_content"`
	EnvContent     string `json:"env_content"`
}

// ActionResponse represents standard output from stack actions (up, down, pull, restart).
type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}
