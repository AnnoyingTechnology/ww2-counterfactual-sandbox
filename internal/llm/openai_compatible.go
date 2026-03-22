package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/config"
)

type OpenAICompatibleClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func NewOpenAICompatibleClient(cfg config.LLMConfig) *OpenAICompatibleClient {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &OpenAICompatibleClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  os.Getenv(cfg.APIKeyEnv),
		model:   cfg.Model,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OpenAICompatibleClient) Name() string {
	return "openai_compatible"
}

func (c *OpenAICompatibleClient) GenerateJSON(ctx context.Context, request StructuredRequest, out any) error {
	if c.baseURL == "" {
		return fmt.Errorf("openai-compatible base URL is required")
	}
	if c.model == "" {
		return fmt.Errorf("model is required")
	}
	if c.apiKey == "" {
		return fmt.Errorf("API key environment variable is empty")
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("llm request failed with status %s", response.Status)
	}

	var parsed chatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return err
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("llm returned no choices")
	}

	content := trimJSONEnvelope(parsed.Choices[0].Message.Content)
	return json.Unmarshal([]byte(content), out)
}

func trimJSONEnvelope(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	return strings.TrimSpace(value)
}
