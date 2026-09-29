package sharedmodels

type NodeTask struct {
	Node                 string            `json:"node"`
	ContainerName        string            `json:"container_name"`
	Image                string            `json:"image"`
	Command              []string          `json:"command"`
	EnvironmentVariables []string          `json:"environment_variables"`
	ExposedPorts         map[string]string `json:"exposed_ports"`
	Volumes              []string          `json:"volumes"`

	MaxCPU    float32 `json:"max_cpu"`
	MaxMemory int64   `json:"max_memory"`
}
