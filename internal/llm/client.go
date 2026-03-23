package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/config"
)

type StructuredRequest struct {
	SystemPrompt string
	UserPrompt   string
	SchemaName   string
	Temperature  float64
	MaxTokens    int
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolRequest struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
	ToolChoice   any
	Tools        []ToolDefinition
}

type ToolCall struct {
	Name      string
	Arguments json.RawMessage
}

type Client interface {
	Name() string
	GenerateJSON(ctx context.Context, request StructuredRequest, out any) error
}

type ToolCallingClient interface {
	Client
	GenerateToolCalls(ctx context.Context, request ToolRequest) ([]ToolCall, error)
}

func NewClient(cfg config.LLMConfig) (Client, error) {
	switch cfg.Provider {
	case "", "mock":
		return nil, nil
	case "openai_compatible":
		return NewOpenAICompatibleClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
