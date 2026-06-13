package balancer

import (
	"sync/atomic"
)

type RoundRobin struct {
	backends []*Backend
	current  uint64
}

func NewRoundRobin(backends []*Backend) *RoundRobin {
	return &RoundRobin{
		backends: backends,
	}
}

func (r *RoundRobin) Next() *Backend {
	n := len(r.backends)
	if n == 0 {
		return nil
	}

	for i := 0; i < n; i++ {
		idx := atomic.AddUint64(&r.current, 1)
		backend := r.backends[idx%uint64(n)]

		if backend.IsAlive() {
			return backend
		}
	}

	return nil
}
