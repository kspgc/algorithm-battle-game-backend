package sandbox

import (
	"sync"

	"github.com/docker/docker/client"
)

type Sandbox struct {
	ID string
}

type Pool struct {
	client      *client.Client
	environment SandboxEnvironmentConfig
	idle        chan *Sandbox

	activeLock sync.Mutex
	active     map[*Sandbox]struct{}
	closed     bool
}
