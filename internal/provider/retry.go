package provider

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type retryProvider struct {
	inner      Provider
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// WithRetry wraps a Provider with exponential-backoff retry on 429/5xx errors.
// maxRetries=0 disables retries. Pass <=0 to use the default (4).
func WithRetry(p Provider, maxRetries int) Provider {
	if maxRetries <= 0 {
		maxRetries = 4
	}
	return &retryProvider{
		inner:      p,
		maxRetries: maxRetries,
		baseDelay:  time.Second,
		maxDelay:   30 * time.Second,
	}
}

func (r *retryProvider) Complete(ctx context.Context, prompt string) (Response, error) {
	return r.do(ctx, func() (Response, error) { return r.inner.Complete(ctx, prompt) })
}

func (r *retryProvider) CompleteMultimodal(ctx context.Context, prompt string, images [][]byte) (Response, error) {
	return r.do(ctx, func() (Response, error) { return r.inner.CompleteMultimodal(ctx, prompt, images) })
}

func (r *retryProvider) do(ctx context.Context, call func() (Response, error)) (Response, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		resp, err := call()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return Response{}, err
		}
		if attempt == r.maxRetries {
			break
		}
		delay := r.backoffDelay(attempt)
		fmt.Printf("[retry] attempt %d/%d after %v (err: %v)\n", attempt+1, r.maxRetries, delay, err)
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	return Response{}, fmt.Errorf("after %d retries: %w", r.maxRetries, lastErr)
}

func (r *retryProvider) backoffDelay(attempt int) time.Duration {
	// exp = base * 2^attempt, capped at max; add jitter in [0, base)
	exp := r.baseDelay << attempt
	if exp <= 0 || exp > r.maxDelay {
		exp = r.maxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(r.baseDelay)))
	return exp + jitter
}

// isRetryable returns true for 429 and 5xx responses from the OpenAI-compatible API.
func isRetryable(err error) bool {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.HTTPStatusCode
		if code == 429 {
			return true
		}
		if code >= 500 && code < 600 {
			return true
		}
		return false
	}
	// Network-level errors from the SDK are wrapped as RequestError.
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		code := reqErr.HTTPStatusCode
		if code == 429 || (code >= 500 && code < 600) {
			return true
		}
	}
	return false
}
