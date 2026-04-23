package task

import (
	"testing"
	"time"
)

func TestHubSubscribePublishReceive(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("task-1")
	defer unsub()

	evt := SSEEvent{Type: "progress", Data: "hello"}
	h.Publish("task-1", evt)

	select {
	case got := <-ch:
		if got.Type != evt.Type {
			t.Fatalf("got type %q, want %q", got.Type, evt.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestHubTwoSubscribersBothReceive(t *testing.T) {
	h := NewHub()
	ch1, unsub1 := h.Subscribe("task-2")
	ch2, unsub2 := h.Subscribe("task-2")
	defer unsub1()
	defer unsub2()

	evt := SSEEvent{Type: "chapter_detected", Data: 42}
	h.Publish("task-2", evt)

	for i, ch := range []<-chan SSEEvent{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Type != evt.Type {
				t.Errorf("subscriber %d got type %q, want %q", i+1, got.Type, evt.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d timed out", i+1)
		}
	}
}

func TestHubDropOnFull(t *testing.T) {
	h := NewHub()
	// Subscribe and never drain the channel (capacity 64).
	_, unsub := h.Subscribe("task-3")
	defer unsub()

	// Fill 64 slots, then publish one more — must not block.
	const bufSize = 64
	for i := 0; i < bufSize; i++ {
		h.Publish("task-3", SSEEvent{Type: "fill"})
	}

	// 65th publish should drop silently, not block.
	done := make(chan struct{})
	go func() {
		h.Publish("task-3", SSEEvent{Type: "overflow"})
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on a full channel")
	}
}

func TestHubCloseRemovesSubscribers(t *testing.T) {
	h := NewHub()
	ch, _ := h.Subscribe("task-4")

	h.Close("task-4")

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after Hub.Close")
	}

	// Further Publish should be a no-op (not block, not panic).
	done := make(chan struct{})
	go func() {
		h.Publish("task-4", SSEEvent{Type: "after-close"})
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish after Close blocked")
	}
}
