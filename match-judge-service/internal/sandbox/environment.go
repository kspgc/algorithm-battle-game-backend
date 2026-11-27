package sandbox

import "github.com/docker/docker/api/types/container"

const defaultWorkingDir = "/workspace"

type SandboxEnvironmentConfig struct {
	Name            string
	ImageName       string
	WorkingDir      string
	EntryFileName   string
	RunCommand      []string
	Cmd             []string
	MemoryLimitMB   int64
	CPUQuotaCores   float64
	MaxProcessCount int64
	NetworkDisabled bool
}

var (
	PythonEnvironment = SandboxEnvironmentConfig{
		Name:            "python",
		ImageName:       "python:3.13-slim",
		WorkingDir:      defaultWorkingDir,
		EntryFileName:   "main.py",
		RunCommand:      []string{"python3", "main.py"},
		Cmd:             []string{"sleep", "infinity"},
		MemoryLimitMB:   128,
		CPUQuotaCores:   0.5,
		MaxProcessCount: 64,
		NetworkDisabled: true,
	}
)

func (config SandboxEnvironmentConfig) toDockerConfigs() (*container.Config, *container.HostConfig) {
	workingDir := config.WorkingDir
	if workingDir == "" {
		workingDir = defaultWorkingDir
	}

	cmd := config.Cmd
	if len(cmd) == 0 {
		cmd = []string{"sleep", "infinity"}
	}

	containerConfig := &container.Config{
		Image:           config.ImageName,
		WorkingDir:      workingDir,
		Cmd:             cmd,
		NetworkDisabled: config.NetworkDisabled,
		Tty:             false,
	}

	pidsLimit := config.MaxProcessCount
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:    config.MemoryLimitMB * 1024 * 1024,
			NanoCPUs:  int64(config.CPUQuotaCores * 1_000_000_000),
			PidsLimit: &pidsLimit,
		},
	}

	return containerConfig, hostConfig
}
