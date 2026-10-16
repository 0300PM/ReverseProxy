package balancer

import (
	"fmt"
	"sync/atomic"
)

type Picker interface {
	Next() *Backend
	Len() int
}

type RoundRobin struct {
	backends []*Backend
	current  atomic.Uint64
}

func NewRoundRobin(backends []*Backend) *RoundRobin {
	return &RoundRobin{backends: backends}
}

func (r *RoundRobin) Len() int {
	return len(r.backends)
}

func (r *RoundRobin) Next() *Backend {
	return pickNth(r.backends, r.current.Add(1))
}

type WeightedRoundRobin struct {
	slots   []*Backend
	current atomic.Uint64
}

func NewWeightedRoundRobin(backends []*Backend) *WeightedRoundRobin {
	slots := make([]*Backend, 0, totalWeight(backends))
	for _, b := range backends {
		for i := 0; i < b.Weight(); i++ {
			slots = append(slots, b)
		}
	}
	return &WeightedRoundRobin{slots: slots}
}

func (w *WeightedRoundRobin) Len() int {
	return len(w.slots)
}

func (w *WeightedRoundRobin) Next() *Backend {
	return pickNth(w.slots, w.current.Add(1))
}

type LeastConnections struct {
	backends []*Backend
	current  atomic.Uint64
}

func NewLeastConnections(backends []*Backend) *LeastConnections {
	return &LeastConnections{backends: backends}
}

func (l *LeastConnections) Len() int {
	return len(l.backends)
}

func (l *LeastConnections) Next() *Backend {
	n := len(l.backends)
	if n == 0 {
		return nil
	}
	start := int(l.current.Add(1) % uint64(n))
	var chosen *Backend
	for i := 0; i < n; i++ {
		b := l.backends[(start+i)%n]
		if !b.Available() {
			continue
		}
		if chosen == nil || b.Inflight() < chosen.Inflight() {
			chosen = b
		}
	}
	return chosen
}

func NewPicker(strategy string, backends []*Backend) (Picker, error) {
	switch strategy {
	case "weighted_round_robin":
		return NewWeightedRoundRobin(backends), nil
	case "least_connections":
		return NewLeastConnections(backends), nil
	case "round_robin":
		return NewRoundRobin(backends), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", strategy)
	}
}

func pickNth(backends []*Backend, nth uint64) *Backend {
	n := len(backends)
	if n == 0 {
		return nil
	}
	start := int(nth % uint64(n))
	for i := 0; i < n; i++ {
		b := backends[(start+i)%n]
		if b.Available() {
			return b
		}
	}
	return nil
}

func totalWeight(backends []*Backend) int {
	total := 0
	for _, b := range backends {
		total += b.Weight()
	}
	return total
}
