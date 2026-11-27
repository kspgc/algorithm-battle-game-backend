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

const cleanupTimeout = 5 * time.Second

func (pool *Pool) createWarmContainer(ctx context.Context) (*Sandbox, error) {
	containerID, err := pool.createContainer(ctx)
	if err != nil {
		return nil, err
	}

	if err := pool.startContainer(ctx, containerID); err != nil {
		pool.cleanupContainer(containerID)
		return nil, err
	}

	return &Sandbox{ID: containerID}, nil
}

func (pool *Pool) createContainer(ctx context.Context) (string, error) {
	containerConfig, hostConfig, err := pool.environment.toDockerConfigs()
	if err != nil {
		return "", fmt.Errorf("build docker configs: %w", err)
	}

	created, err := pool.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("create %s container: %w", pool.environment.Name, err)
	}

	return created.ID, nil
}

func (pool *Pool) startContainer(ctx context.Context, containerID string) error {
	if err := pool.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start %s container %s: %w", pool.environment.Name, containerID, err)
	}
	return nil
}

func (pool *Pool) cleanupContainer(containerID string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	_ = pool.client.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true})
}
