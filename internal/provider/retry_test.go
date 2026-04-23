package provider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// flakyProvider returns the configured error for the first `failN` calls,
// then returns a success Response.
type flakyProvider struct {
	failN    int
	err      error
	attempts atomic.Int32
}

func (f *flakyProvider) Complete(_ context.Context, _ string) (Response, error) {
	n := int(f.attempts.Add(1))
	if n <= f.failN {
		return Response{}, f.err
	}
	return Response{Content: "ok"}, nil
}

func (f *flakyProvider) CompleteJSON(ctx context.Context, prompt string) (Response, error) {
	return f.Complete(ctx, prompt)
}

func (f *flakyProvider) CompleteMultimodal(_ context.Context, _ string, _ [][]byte) (Response, error) {
	return f.Complete(context.Background(), "")
}

func TestRetry_SucceedsAfter429(t *testing.T) {
	f := &flakyProvider{failN: 2, err: &openai.APIError{HTTPStatusCode: 429, Message: "rate limited"}}
	r := &retryProvider{inner: f, maxRetries: 4, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}

	resp, err := r.Complete(context.Background(), "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q; want %q", resp.Content, "ok")
	}
	if got := f.attempts.Load(); got != 3 {
		t.Errorf("attempts = %d; want 3", got)
	}
}

func TestRetry_ExhaustsOn429(t *testing.T) {
	f := &flakyProvider{failN: 100, err: &openai.APIError{HTTPStatusCode: 429, Message: "rate limited"}}
	r := &retryProvider{inner: f, maxRetries: 2, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}

	_, err := r.Complete(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := f.attempts.Load(); got != 3 { // initial + 2 retries
		t.Errorf("attempts = %d; want 3", got)
	}
}

func TestRetry_DoesNotRetry400(t *testing.T) {
	f := &flakyProvider{failN: 10, err: &openai.APIError{HTTPStatusCode: 400, Message: "bad request"}}
	r := &retryProvider{inner: f, maxRetries: 4, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}

	_, err := r.Complete(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := f.attempts.Load(); got != 1 {
		t.Errorf("attempts = %d; want 1 (non-retryable)", got)
	}
}

func TestRetry_DoesNotRetryNonAPIError(t *testing.T) {
	f := &flakyProvider{failN: 10, err: errors.New("some local error")}
	r := &retryProvider{inner: f, maxRetries: 4, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}

	_, err := r.Complete(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := f.attempts.Load(); got != 1 {
		t.Errorf("attempts = %d; want 1 (non-retryable)", got)
	}
}

func TestRetry_CtxCancellation(t *testing.T) {
	f := &flakyProvider{failN: 100, err: &openai.APIError{HTTPStatusCode: 429, Message: "rate limited"}}
	// baseDelay is long so ctx cancellation deterministically hits during the first sleep.
	r := &retryProvider{inner: f, maxRetries: 10, baseDelay: 5 * time.Second, maxDelay: 30 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := r.Complete(ctx, "p")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
