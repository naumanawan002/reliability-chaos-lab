package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	// Threshold = 2 failures, 50ms recovery timeout
	cb := NewBreaker(2, 50*time.Millisecond)

	// 1. Initial State: CLOSED
	if cb.State() != StateClosed {
		t.Fatalf("expected initial state CLOSED, got %s", cb.State())
	}

	dummyErr := errors.New("upstream timeout")

	// 2. Failure 1 -> still CLOSED
	_ = cb.Execute(func() error { return dummyErr })
	if cb.State() != StateClosed {
		t.Errorf("expected state CLOSED after 1 fail, got %s", cb.State())
	}

	// 3. Failure 2 -> Threshold reached! Must trip to OPEN
	_ = cb.Execute(func() error { return dummyErr })
	if cb.State() != StateOpen {
		t.Fatalf("expected state OPEN after 2 fails, got %s", cb.State())
	}

	// 4. In OPEN state, calls must fast-fail without executing function
	executed := false
	err := cb.Execute(func() error {
		executed = true
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if executed {
		t.Error("protected function should NOT have executed while breaker is OPEN")
	}

	// 5. Wait for recovery timeout (60ms) -> transitions to HALF_OPEN on next probe
	time.Sleep(60 * time.Millisecond)

	// Trial call in HALF_OPEN succeeds -> Breaker resets to CLOSED
	errRecover := cb.Execute(func() error {
		return nil // Success!
	})
	if errRecover != nil {
		t.Fatalf("recovery call failed: %v", errRecover)
	}
	if cb.State() != StateClosed {
		t.Errorf("expected breaker to reset to CLOSED, got %s", cb.State())
	}
}
