package llm

import (
	"context"
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

type Client interface {
	Name() string
	GenerateJSON(ctx context.Context, request StructuredRequest, out any) error
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
