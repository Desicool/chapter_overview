package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

const (
	kimiBaseURL       = "https://api.moonshot.cn/v1"
	kimiDefaultText   = "moonshot-v1-32k"
	kimiDefaultVision = "moonshot-v1-32k-vision-preview"
)

type kimiProvider struct {
	client      *openai.Client
	textModel   string
	visionModel string
}

func newKimi(cfg Config) (Provider, error) {
	apiKey := os.Getenv("MOONSHOT_API_KEY")
	if apiKey == "" {
		return nil, errors.New("MOONSHOT_API_KEY environment variable is not set")
	}

	conf := openai.DefaultConfig(apiKey)
	conf.BaseURL = kimiBaseURL

	textModel := cfg.TextModel
	if textModel == "" {
		textModel = kimiDefaultText
	}
	visionModel := cfg.VisionModel
	if visionModel == "" {
		visionModel = kimiDefaultVision
	}

	return &kimiProvider{
		client:      openai.NewClientWithConfig(conf),
		textModel:   textModel,
		visionModel: visionModel,
	}, nil
}

func init() {
	Register("kimi", newKimi)
}

// Complete sends a text-only prompt.
func (k *kimiProvider) Complete(ctx context.Context, prompt string) (Response, error) {
	resp, err := k.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: k.textModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return Response{}, fmt.Errorf("kimi complete: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Response{}, errors.New("kimi complete: empty response")
	}
	return Response{
		Content: resp.Choices[0].Message.Content,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

// CompleteMultimodal sends text + images to the vision model.
func (k *kimiProvider) CompleteMultimodal(ctx context.Context, prompt string, images [][]byte) (Response, error) {
	parts := []openai.ChatMessagePart{
		{Type: openai.ChatMessagePartTypeText, Text: prompt},
	}

	for _, img := range images {
		b64 := base64.StdEncoding.EncodeToString(img)
		dataURL := "data:image/jpeg;base64," + b64
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: dataURL,
			},
		})
	}

	resp, err := k.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: k.visionModel,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:         openai.ChatMessageRoleUser,
				MultiContent: parts,
			},
		},
	})
	if err != nil {
		return Response{}, fmt.Errorf("kimi multimodal: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Response{}, errors.New("kimi multimodal: empty response")
	}
	return Response{
		Content: resp.Choices[0].Message.Content,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}
