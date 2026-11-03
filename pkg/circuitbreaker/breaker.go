package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is OPEN: fast failing call to prevent cascading failure")
)

type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

// Breaker protects downstream calls from cascading thread exhaustion.
type Breaker struct {
	mu               sync.RWMutex
	state            State
	failureThreshold int
	consecutiveFails int
	recoveryTimeout  time.Duration
	lastStateChange  time.Time
}

func NewBreaker(failureThreshold int, recoveryTimeout time.Duration) *Breaker {
	return &Breaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
		lastStateChange:  time.Now(),
	}
}

func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Execute runs the protected operation through the circuit breaker state machine.
func (b *Breaker) Execute(fn func() error) error {
	if err := b.beforeCall(); err != nil {
		return err
	}

	err := fn()
	b.afterCall(err)
	return err
}

func (b *Breaker) beforeCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.state == StateOpen {
		if now.Sub(b.lastStateChange) > b.recoveryTimeout {
			// Trial single request in Half-Open state
			b.state = StateHalfOpen
			b.lastStateChange = now
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}

func (b *Breaker) afterCall(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if err != nil {
		b.consecutiveFails++
		if b.state == StateHalfOpen || b.consecutiveFails >= b.failureThreshold {
			// Trip the breaker
			b.state = StateOpen
			b.lastStateChange = now
		}
	} else {
		// Success!
		if b.state == StateHalfOpen {
			b.state = StateClosed
			b.consecutiveFails = 0
			b.lastStateChange = now
		} else if b.state == StateClosed {
			b.consecutiveFails = 0
		}
	}
}
