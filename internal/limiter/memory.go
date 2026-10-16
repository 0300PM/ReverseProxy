package limiter

import (
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type Memory struct {
	mu       sync.Mutex
	clients  map[string]*entry
	rps      rate.Limit
	burst    int
	idleTTL  time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64
}

func NewMemory(requestsPerMinute, burst int, idleTTL time.Duration) *Memory {
	m := &Memory{
		clients: make(map[string]*entry),
		rps:     rate.Limit(float64(requestsPerMinute) / 60.0),
		burst:   burst,
		idleTTL: idleTTL,
		stop:    make(chan struct{}),
	}
	go m.janitor()
	return m
}

func (m *Memory) Allow(key string) (bool, error) {
	e := m.get(key)
	e.lastSeen.Store(time.Now().UnixNano())
	return e.limiter.Allow(), nil
}

func (m *Memory) Close() error {
	m.stopOnce.Do(func() { close(m.stop) })
	return nil
}

func (m *Memory) get(key string) *entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.clients[key]; ok {
		return e
	}
	e := &entry{limiter: rate.NewLimiter(m.rps, m.burst)}
	e.lastSeen.Store(time.Now().UnixNano())
	m.clients[key] = e
	return e
}

func (m *Memory) janitor() {
	ticker := time.NewTicker(m.idleTTL)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.evict(now)
		}
	}
}

func (m *Memory) evict(now time.Time) {
	cutoff := now.Add(-m.idleTTL).UnixNano()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.clients {
		if e.lastSeen.Load() < cutoff {
			delete(m.clients, k)
		}
	}
}
