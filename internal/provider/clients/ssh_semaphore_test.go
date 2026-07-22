package clients

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAcquireBoundsConcurrency(t *testing.T) {
	// Arrange
	const limit = 3

	c := &sshMachineAccessClient{sem: make(chan struct{}, limit)}

	var mu sync.Mutex

	var current, observedMax int

	var wg sync.WaitGroup

	// Act
	for i := 0; i < 50; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			release, err := c.acquire(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			defer release()

			mu.Lock()

			current++
			if current > observedMax {
				observedMax = current
			}

			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()

			current--

			mu.Unlock()
		}()
	}

	wg.Wait()

	// Assert
	if observedMax > limit {
		t.Fatalf("observed %d concurrent sessions, want <= %d", observedMax, limit)
	}
}

func TestAcquireUnlimitedWhenSemNil(t *testing.T) {
	// Arrange
	c := &sshMachineAccessClient{sem: nil}

	// Act
	release, err := c.acquire(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	release()
}

func TestAcquireRespectsContext(t *testing.T) {
	// Arrange: a full semaphore so the next acquire must wait
	c := &sshMachineAccessClient{sem: make(chan struct{}, 1)}
	c.sem <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, err := c.acquire(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestWithSessionSlotReleasesOnReturn(t *testing.T) {
	// Arrange
	c := &sshMachineAccessClient{sem: make(chan struct{}, 1)}

	// Act: run twice sequentially; the second call proves the slot from the first
	// was released.
	for i := 0; i < 2; i++ {
		if err := c.withSessionSlot(context.Background(), func() error {
			return nil
		}); err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
	}

	// Assert: the semaphore is empty (a non-blocking send succeeds).
	select {
	case c.sem <- struct{}{}:
	default:
		t.Fatal("session slot was not released")
	}
}
