package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/config"
)

type OpenAICompatibleClient struct {
	baseURL            string
	apiKey             string
	apiKeyEnv          string
	model              string
	responseFormatType string
	client             *http.Client
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Tools          []chatTool        `json:"tools,omitempty"`
	ToolChoice     any               `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Arguments   string         `json:"arguments,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

func NewOpenAICompatibleClient(cfg config.LLMConfig) *OpenAICompatibleClient {
	var timeout time.Duration
	switch {
	case cfg.TimeoutSeconds < 0:
		timeout = 0
	case cfg.TimeoutSeconds == 0:
		timeout = 60 * time.Second
	default:
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	httpClient := &http.Client{}
	if timeout > 0 {
		httpClient.Timeout = timeout
	}

	return &OpenAICompatibleClient{
		baseURL:            strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:             os.Getenv(cfg.APIKeyEnv),
		apiKeyEnv:          cfg.APIKeyEnv,
		model:              cfg.Model,
		responseFormatType: cfg.ResponseFormatType,
		client:             httpClient,
	}
}

func (c *OpenAICompatibleClient) Name() string {
	return "openai_compatible"
}

func (c *OpenAICompatibleClient) GenerateJSON(ctx context.Context, request StructuredRequest, out any) error {
	if err := c.validate(); err != nil {
		return err
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
	}
	if c.responseFormatType != "" && c.responseFormatType != "none" {
		payload.ResponseFormat = map[string]string{
			"type": c.responseFormatType,
		}
	}

	parsed, err := c.doChatCompletion(ctx, payload)
	if err != nil {
		return err
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("llm returned no choices")
	}

	choice := parsed.Choices[0]
	content := trimJSONEnvelope(choice.Message.Content)
	if content == "" && strings.TrimSpace(choice.Message.ReasoningContent) != "" {
		return fmt.Errorf("llm returned no final content; reasoning consumed the budget (finish_reason=%s): %s", choice.FinishReason, excerpt(choice.Message.ReasoningContent, 600))
	}
	if err := json.Unmarshal([]byte(content), out); err == nil {
		return nil
	}

	if extracted := extractJSONObject(content); extracted != "" && extracted != content {
		if err := json.Unmarshal([]byte(extracted), out); err == nil {
			return nil
		}
	}

	return fmt.Errorf("llm returned invalid JSON: %s", excerpt(content, 600))
}

func (c *OpenAICompatibleClient) GenerateToolCalls(ctx context.Context, request ToolRequest) ([]ToolCall, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		ToolChoice:  request.ToolChoice,
	}
	if payload.ToolChoice == nil {
		payload.ToolChoice = "auto"
	}
	for _, tool := range request.Tools {
		payload.Tools = append(payload.Tools, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	parsed, err := c.doChatCompletion(ctx, payload)
	if err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	choice := parsed.Choices[0]
	if len(choice.Message.ToolCalls) == 0 {
		content := trimJSONEnvelope(choice.Message.Content)
		if content == "" && strings.TrimSpace(choice.Message.ReasoningContent) != "" {
			return nil, fmt.Errorf("llm returned no tool calls and no final content; reasoning consumed the budget (finish_reason=%s): %s", choice.FinishReason, excerpt(choice.Message.ReasoningContent, 600))
		}
		return nil, fmt.Errorf("llm returned no tool calls: %s", excerpt(content, 600))
	}

	calls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, toolCall := range choice.Message.ToolCalls {
		args := trimJSONEnvelope(toolCall.Function.Arguments)
		if extracted := extractJSONObject(args); extracted != "" {
			args = extracted
		}
		if args == "" {
			args = "{}"
		}

		var scratch json.RawMessage
		if err := json.Unmarshal([]byte(args), &scratch); err != nil {
			return nil, fmt.Errorf("llm returned invalid tool arguments for %s: %s", toolCall.Function.Name, excerpt(args, 400))
		}

		calls = append(calls, ToolCall{
			Name:      toolCall.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}

	return calls, nil
}

func (c *OpenAICompatibleClient) validate() error {
	if c.baseURL == "" {
		return fmt.Errorf("openai-compatible base URL is required")
	}
	if c.model == "" {
		return fmt.Errorf("model is required")
	}
	if c.apiKeyEnv != "" && c.apiKey == "" {
		return fmt.Errorf("API key environment variable %q is empty", c.apiKeyEnv)
	}
	return nil
}

func (c *OpenAICompatibleClient) doChatCompletion(ctx context.Context, payload chatCompletionRequest) (chatCompletionResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return chatCompletionResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatCompletionResponse{}, err
	}
	if c.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(httpRequest)
	if err != nil {
		return chatCompletionResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if len(body) == 0 {
			return chatCompletionResponse{}, fmt.Errorf("llm request failed with status %s", response.Status)
		}
		return chatCompletionResponse{}, fmt.Errorf("llm request failed with status %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var parsed chatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return chatCompletionResponse{}, err
	}

	return parsed, nil
}

func trimJSONEnvelope(value string) string {
	value = stripReasoningBlocks(value)
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```JSON")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	return strings.TrimSpace(value)
}

var reasoningBlockPattern = regexp.MustCompile(`(?is)<\s*(think|thinking)\b[^>]*>.*?<\s*/\s*(think|thinking)\s*>`)

func stripReasoningBlocks(value string) string {
	cleaned := reasoningBlockPattern.ReplaceAllString(value, "")
	return strings.TrimSpace(cleaned)
}

func extractJSONObject(value string) string {
	start := strings.IndexAny(value, "{[")
	if start == -1 {
		return ""
	}

	var stack []rune
	for index, char := range value[start:] {
		switch char {
		case '{', '[':
			stack = append(stack, char)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return ""
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return ""
			}
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			return strings.TrimSpace(value[start : start+index+1])
		}
	}

	return ""
}

func excerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
