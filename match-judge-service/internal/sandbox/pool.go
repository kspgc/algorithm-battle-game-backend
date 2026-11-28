package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"
)

const (
	maxCreateRetries   = 5
	retryDelay         = 2 * time.Second
	initCleanupTimeout = 15 * time.Second
)

var (
	ErrPoolClosed      = errors.New("sandbox pool is closed")
	ErrInvalidPoolSize = errors.New("pool size must be greater than 0")
)

type Pool struct {
	client      *client.Client
	environment SandboxEnvironmentConfig
	idle        chan *Sandbox

	activeLock sync.Mutex
	active     map[*Sandbox]struct{}
	closed     bool
}

func NewPool(ctx context.Context, dockerClient *client.Client, environment SandboxEnvironmentConfig, size int) (*Pool, error) {
	if err := validatePoolConfig(dockerClient, environment, size); err != nil {
		return nil, err
	}

	pool := &Pool{
		client:      dockerClient,
		environment: environment,
		idle:        make(chan *Sandbox, size),
		active:      make(map[*Sandbox]struct{}, size),
	}

	if err := pool.warmup(ctx, size); err != nil {
		pool.cleanupAfterFailedInit()
		return nil, fmt.Errorf("warmup %s pool: %w", environment.Name, err)
	}

	return pool, nil
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

func (pool *Pool) Close(ctx context.Context) error {
	targets, ok := pool.closeAndDrain()
	if !ok {
		return nil
	}

	return pool.destroyAll(ctx, targets)
}

// === === === === === === === === === === === === === === === === === === === ===

func validatePoolConfig(dockerClient *client.Client, environment SandboxEnvironmentConfig, size int) error {
	if dockerClient == nil {
		return errors.New("docker client cannot be nil")
	}
	if size < 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidPoolSize, size)
	}
	if err := environment.validate(); err != nil {
		return fmt.Errorf("invalid environment: %w", err)
	}
	return nil
}

func (pool *Pool) warmup(ctx context.Context, count int) error {
	group, warmupCtx := errgroup.WithContext(ctx)

	for range count {
		group.Go(func() error {
			return pool.warmupSingle(warmupCtx)
		})
	}

	return group.Wait()
}

func (pool *Pool) warmupSingle(ctx context.Context) error {
	sandbox, err := pool.createWarmContainer(ctx)
	if err != nil {
		return err
	}

	if !pool.register(sandbox) {
		pool.cleanupContainer(sandbox.ID)
		return nil
	}

	select {
	case pool.idle <- sandbox:
		return nil
	case <-ctx.Done():
		pool.cleanupContainer(sandbox.ID)
		return ctx.Err()
	}
}

func (pool *Pool) cleanupAfterFailedInit() {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), initCleanupTimeout)
	defer cancel()

	_ = pool.Close(cleanupCtx)
}

func (pool *Pool) register(sandbox *Sandbox) bool {
	pool.activeLock.Lock()
	defer pool.activeLock.Unlock()

	if pool.closed {
		return false
	}
	pool.active[sandbox] = struct{}{}
	return true
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

	for range maxCreateRetries {
		if pool.isClosed() {
			return
		}

		newSandbox, err := pool.tryCreateWarmContainer()
		if err == nil {
			if !pool.registerAndEnqueue(newSandbox) {
				pool.cleanupContainer(newSandbox.ID)
			}
			return
		}
	}

	// TODO: 可在此接入專案的 Logger 記錄補建失敗

	time.Sleep(retryDelay)
}
