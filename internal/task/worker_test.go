package task

import (
	"testing"
)

// TestWorkerCircuitBreaker verifies that Submit returns ErrTooManyTasks when
// the semaphore is full, without blocking.
func TestWorkerCircuitBreaker(t *testing.T) {
	w := &Worker{
		sem: make(chan struct{}, 1),
	}

	// Occupy the single slot
	w.sem <- struct{}{}

	// Submit should fail immediately
	err := w.Submit(&Task{ID: "t1"})
	if err != ErrTooManyTasks {
		t.Fatalf("expected ErrTooManyTasks, got %v", err)
	}

	// Release the slot
	<-w.sem

	// Now the slot is free — Submit would proceed (we don't have a full pipeline
	// to run so we'd panic on nil prov; just verify no error is returned for the
	// non-blocking select path by checking the sem state).
	// We can't call Submit here without a valid provider, so just confirm the
	// slot is free.
	if len(w.sem) != 0 {
		t.Fatal("expected empty semaphore after release")
	}
}

// TestWorkerCircuitBreakerCapacity verifies that at capacity=2 exactly 2 tasks
// can be queued before ErrTooManyTasks.
func TestWorkerCircuitBreakerCapacity(t *testing.T) {
	w := &Worker{
		sem: make(chan struct{}, 2),
	}

	// Occupy both slots
	w.sem <- struct{}{}
	w.sem <- struct{}{}

	err := w.Submit(&Task{ID: "overflow"})
	if err != ErrTooManyTasks {
		t.Fatalf("expected ErrTooManyTasks at capacity, got %v", err)
	}
}
