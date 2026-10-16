package breaker

import (
	"testing"
	"time"
)

func TestClosedToOpenToClosed(t *testing.T) {
	b := New(3, 50*time.Millisecond, 1)
	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatalf("allow %d: expected true", i)
		}
		b.RecordFailure()
	}
	if b.State() != Closed {
		t.Fatalf("state = %v, want closed", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected allow before threshold")
	}
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("state = %v, want open", b.State())
	}
	if b.Allow() {
		t.Fatal("expected deny while open")
	}
	time.Sleep(60 * time.Millisecond)
	if b.State() != HalfOpen {
		t.Fatalf("state = %v, want half-open", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected one probe in half-open")
	}
	if b.Allow() {
		t.Fatal("expected probe limit in half-open")
	}
	b.RecordSuccess()
	if b.State() != Closed {
		t.Fatalf("state = %v, want closed after success", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected allow after close")
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	b := New(1, 20*time.Millisecond, 1)
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("state = %v, want open", b.State())
	}
	time.Sleep(25 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected probe")
	}
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("state = %v, want open after half-open failure", b.State())
	}
	if b.Allow() {
		t.Fatal("expected deny after re-open")
	}
}

func TestSuccessResetsFailures(t *testing.T) {
	b := New(2, time.Minute, 1)
	b.RecordFailure()
	b.RecordSuccess()
	b.RecordFailure()
	if b.State() != Closed {
		t.Fatalf("state = %v, want closed", b.State())
	}
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("state = %v, want open", b.State())
	}
}

func TestOnStateChange(t *testing.T) {
	b := New(1, 10*time.Millisecond, 1)
	var states []State
	b.OnStateChange(func(s State) { states = append(states, s) })
	b.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	_ = b.State()
	b.Allow()
	b.RecordSuccess()
	want := []State{Open, HalfOpen, Closed}
	if len(states) != len(want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	for i := range want {
		if states[i] != want[i] {
			t.Fatalf("states = %v, want %v", states, want)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	b := New(5, time.Millisecond, 2)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				if b.Allow() {
					if j%2 == 0 {
						b.RecordSuccess()
					} else {
						b.RecordFailure()
					}
				}
				_ = b.State()
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
