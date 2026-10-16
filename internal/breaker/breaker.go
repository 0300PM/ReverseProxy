package breaker

import (
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type Breaker struct {
	mu               sync.Mutex
	failureThreshold int
	openTimeout      time.Duration
	halfOpenProbes   int
	state            State
	failures         int
	successes        int
	probes           int
	openedAt         time.Time
	onStateChange    func(State)
}

func New(failureThreshold int, openTimeout time.Duration, halfOpenProbes int) *Breaker {
	return &Breaker{
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
		halfOpenProbes:   halfOpenProbes,
	}
}

func (b *Breaker) OnStateChange(fn func(State)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onStateChange = fn
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transition(time.Now())
	return b.state
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.transition(now)
	switch b.state {
	case Closed:
		return true
	case HalfOpen:
		if b.probes < b.halfOpenProbes {
			b.probes++
			return true
		}
		return false
	default:
		return false
	}
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transition(time.Now())
	switch b.state {
	case HalfOpen:
		b.successes++
		if b.successes >= b.halfOpenProbes {
			b.setState(Closed)
			b.failures = 0
			b.successes = 0
			b.probes = 0
		}
	case Closed:
		b.failures = 0
	}
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transition(time.Now())
	switch b.state {
	case HalfOpen:
		b.setState(Open)
		b.openedAt = time.Now()
		b.failures = 0
		b.successes = 0
		b.probes = 0
	case Closed:
		b.failures++
		if b.failures >= b.failureThreshold {
			b.setState(Open)
			b.openedAt = time.Now()
			b.failures = 0
			b.probes = 0
		}
	}
}

func (b *Breaker) transition(now time.Time) {
	if b.state == Open && now.Sub(b.openedAt) >= b.openTimeout {
		b.setState(HalfOpen)
		b.successes = 0
		b.probes = 0
	}
}

func (b *Breaker) setState(s State) {
	if b.state == s {
		return
	}
	b.state = s
	if b.onStateChange != nil {
		b.onStateChange(s)
	}
}
