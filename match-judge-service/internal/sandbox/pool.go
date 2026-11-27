package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Pool struct {
	client      *client.Client
	environment SandboxEnvironmentConfig
	idle        chan *Sandbox

	activeLock sync.Mutex
	active     map[*Sandbox]struct{}
	closed     bool
}

func (pool *Pool) createWarmContainer(ctx context.Context) (*Sandbox, error) {
	containerConfig, hostConfig, err := pool.environment.toDockerConfigs()
	if err != nil {
		return nil, fmt.Errorf("build docker configs: %w", err)
	}

	created, err := pool.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("create %s container: %w", pool.environment.Name, err)
	}

	if err := pool.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pool.client.ContainerRemove(cleanupCtx, created.ID, container.RemoveOptions{Force: true})

		return nil, fmt.Errorf("start %s container %s: %w", pool.environment.Name, created.ID, err)
	}

	return &Sandbox{ID: created.ID}, nil
}
