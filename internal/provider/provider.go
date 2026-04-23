package provider

import (
	"context"
	"fmt"
)

// Usage holds token counts from an LLM API call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Response wraps LLM output with token usage.
type Response struct {
	Content string
	Usage   Usage
}

// Provider is the interface all LLM backends must implement.
type Provider interface {
	// Complete sends a text-only prompt; returns prose.
	Complete(ctx context.Context, prompt string) (Response, error)
	// CompleteJSON sends a prompt and forces JSON output using the provider's
	// detection model (typically smaller/faster than the summarization model).
	CompleteJSON(ctx context.Context, prompt string) (Response, error)
	// CompleteMultimodal sends text + images to the vision model.
	CompleteMultimodal(ctx context.Context, prompt string, images [][]byte) (Response, error)
}

// Config holds model overrides for a provider.
type Config struct {
	TextModel   string
	VisionModel string
	DetectModel string // lightweight model for detection/merging; defaults to provider default
}

type factory func(cfg Config) (Provider, error)

var registry = map[string]factory{}

func Register(name string, f factory) {
	registry[name] = f
}

func Get(name string, cfg Config) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		available := make([]string, 0, len(registry))
		for k := range registry {
			available = append(available, k)
		}
		return nil, fmt.Errorf("unknown provider %q — available: %v", name, available)
	}
	return f(cfg)
}
