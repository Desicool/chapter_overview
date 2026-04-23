package provider

import (
	"context"
	"testing"
)

// mockProvider implements Provider for testing.
type mockProvider struct{}

func (m *mockProvider) Complete(_ context.Context, _ string) (Response, error) {
	return Response{
		Content: "ok",
		Usage:   Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (m *mockProvider) CompleteMultimodal(_ context.Context, _ string, _ [][]byte) (Response, error) {
	return Response{
		Content: "ok",
		Usage:   Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func TestRegisterAndGet(t *testing.T) {
	// Register mock provider
	Register("mock", func(cfg Config) (Provider, error) {
		return &mockProvider{}, nil
	})

	// Get registered provider — should succeed
	prov, err := Get("mock", Config{})
	if err != nil {
		t.Fatalf("Get(\"mock\") unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("Get(\"mock\") returned nil provider")
	}

	// Verify Complete returns expected Response
	resp, err := prov.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete() unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Complete() Content = %q; want %q", resp.Content, "ok")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Complete() InputTokens = %d; want 10", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("Complete() OutputTokens = %d; want 5", resp.Usage.OutputTokens)
	}
}

func TestGetUnknownProvider(t *testing.T) {
	_, err := Get("does-not-exist", Config{})
	if err == nil {
		t.Fatal("Get(\"does-not-exist\") expected error, got nil")
	}
}
