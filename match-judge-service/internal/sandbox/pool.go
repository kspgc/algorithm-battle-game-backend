package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const (
	createTimeout  = 30 * time.Second
	cleanupTimeout = 5 * time.Second
)

var (
	ErrPoolClosed     = errors.New("sandbox pool is closed")
	ErrUnownedSandbox = errors.New("sandbox does not belong to this pool")
)

type Pool struct {
	client      *client.Client
	environment SandboxEnvironmentConfig
	idle        chan *Sandbox

	activeLock sync.Mutex
	active     map[*Sandbox]struct{}
	closed     bool
}

func (pool *Pool) Acquire(ctx context.Context) (*Sandbox, error) {
	if pool.isClosed() {
		return nil, ErrPoolClosed
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("acquire sandbox: %w", ctx.Err())
	case sandbox, ok := <-pool.idle:
		if !ok || sandbox == nil {
			return nil, ErrPoolClosed
		}
		return sandbox, nil
	}
}

func (pool *Pool) Release(sandbox *Sandbox) {
	if sandbox == nil {
		return
	}

	if !pool.unregister(sandbox) {
		return
	}

	go pool.replaceSandbox(sandbox)
}

// === === === === === === === === === === === === === === === === === === === ===

func (pool *Pool) Close(ctx context.Context) error {
	targets, ok := pool.closeAndDrain()
	if !ok {
		return nil
	}

	return pool.destroyAll(ctx, targets)
}

func (pool *Pool) isClosed() bool {
	pool.activeLock.Lock()
	defer pool.activeLock.Unlock()
	return pool.closed
}

func (pool *Pool) registerAndEnqueue(sandbox *Sandbox) bool {
	pool.activeLock.Lock()
	defer pool.activeLock.Unlock()

	if pool.closed {
		return false
	}

	pool.active[sandbox] = struct{}{}
	pool.idle <- sandbox
	return true
}

func (pool *Pool) unregister(sandbox *Sandbox) bool {
	pool.activeLock.Lock()
	defer pool.activeLock.Unlock()

	if _, exists := pool.active[sandbox]; !exists {
		return false
	}

	delete(pool.active, sandbox)
	return true
}

func (pool *Pool) closeAndDrain() ([]*Sandbox, bool) {
	pool.activeLock.Lock()
	defer pool.activeLock.Unlock()

	if pool.closed {
		return nil, false
	}

	pool.closed = true
	close(pool.idle)

	targets := make([]*Sandbox, 0, len(pool.active))
	for sandbox := range pool.active {
		targets = append(targets, sandbox)
	}
	clear(pool.active)

	return targets, true
}

func (pool *Pool) replaceSandbox(oldSandbox *Sandbox) {
	pool.cleanupContainer(oldSandbox.ID)

	if pool.isClosed() {
		return
	}

	createCtx, cancel := context.WithTimeout(context.Background(), createTimeout)
	defer cancel()

	newSandbox, err := pool.createWarmContainer(createCtx)
	if err != nil {
		// TODO: 可在此接入專案的 Logger 記錄補建失敗
		return
	}

	if !pool.registerAndEnqueue(newSandbox) {
		pool.cleanupContainer(newSandbox.ID)
	}
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
