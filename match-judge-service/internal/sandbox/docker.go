package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
)

const (
	createTimeout  = 30 * time.Second
	cleanupTimeout = 5 * time.Second
)

func (pool *Pool) tryCreateWarmContainer() (*Sandbox, error) {
	createCtx, cancel := context.WithTimeout(context.Background(), createTimeout)
	defer cancel()

	return pool.createWarmContainer(createCtx)
}

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

func (pool *Pool) destroyAll(ctx context.Context, targets []*Sandbox) error {
	var (
		waitGroup sync.WaitGroup
		errLock   sync.Mutex
		errs      []error
	)

	for _, sandbox := range targets {
		waitGroup.Add(1)
		go func(target *Sandbox) {
			defer waitGroup.Done()

			if err := pool.client.ContainerRemove(ctx, target.ID, container.RemoveOptions{Force: true}); err != nil {
				errLock.Lock()
				errs = append(errs, fmt.Errorf("remove container %s: %w", target.ID, err))
				errLock.Unlock()
			}
		}(sandbox)
	}

	waitGroup.Wait()
	return errors.Join(errs...)
}
