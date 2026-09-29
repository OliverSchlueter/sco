package sharedmodels

type NodeTask struct {
	ContainerName        string            `json:"container_name"`
	Image                string            `json:"image"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	ExposedPorts         map[string]string `json:"exposed_ports"`

	MaxCPU    float32 `json:"max_cpu"`
	MaxMemory int64   `json:"max_memory"`
}
