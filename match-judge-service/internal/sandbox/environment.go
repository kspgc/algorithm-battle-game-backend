package sandbox

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
)

const (
	bytesPerMB        = 1024 * 1024
	nanoCPUsPerCore   = 1_000_000_000
	defaultWorkingDir = "/workspace"
)

var (
	ErrEmptyImageName     = errors.New("image name cannot be empty")
	ErrInvalidMemoryLimit = errors.New("memory limit must be greater than or equal to 0")
	ErrInvalidCPUQuota    = errors.New("cpu quota must be greater than or equal to 0")
	ErrInvalidPidsLimit   = errors.New("max process count must be greater than or equal to 0")
)

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

func (config SandboxEnvironmentConfig) toDockerConfigs() (*container.Config, *container.HostConfig, error) {
	if err := config.validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid sandbox config: %w", err)
	}

	return config.buildContainerConfig(), config.buildHostConfig(), nil
}

func (config SandboxEnvironmentConfig) validate() error {
	if strings.TrimSpace(config.ImageName) == "" {
		return ErrEmptyImageName
	}
	if config.MemoryLimitMB < 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidMemoryLimit, config.MemoryLimitMB)
	}
	if config.CPUQuotaCores < 0 {
		return fmt.Errorf("%w: got %f", ErrInvalidCPUQuota, config.CPUQuotaCores)
	}
	if config.MaxProcessCount < 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidPidsLimit, config.MaxProcessCount)
	}
	return nil
}

func (config SandboxEnvironmentConfig) buildContainerConfig() *container.Config {
	return &container.Config{
		Image:           config.ImageName,
		WorkingDir:      config.resolvedWorkingDir(),
		Cmd:             config.resolvedCmd(),
		NetworkDisabled: config.NetworkDisabled,
		Tty:             false,
	}
}

func (config SandboxEnvironmentConfig) buildHostConfig() *container.HostConfig {
	var resources container.Resources

	if config.MemoryLimitMB > 0 {
		resources.Memory = config.MemoryLimitMB * bytesPerMB
	}
	if config.CPUQuotaCores > 0 {
		resources.NanoCPUs = int64(config.CPUQuotaCores * nanoCPUsPerCore)
	}
	if config.MaxProcessCount > 0 {
		pidsLimit := config.MaxProcessCount
		resources.PidsLimit = &pidsLimit
	}

	return &container.HostConfig{
		Resources: resources,
	}
}

func (config SandboxEnvironmentConfig) resolvedWorkingDir() string {
	if config.WorkingDir == "" {
		return defaultWorkingDir
	}
	return config.WorkingDir
}

func (config SandboxEnvironmentConfig) resolvedCmd() []string {
	if len(config.Cmd) == 0 {
		return []string{"sleep", "infinity"}
	}
	return config.Cmd
}
