package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ClaudeClient is the LLM client — now backed by NVIDIA's OpenAI-compatible API.
// The name is kept so all callers (advisor, optimizer, explainer) compile unchanged.
type ClaudeClient struct {
	apiKey     string
	httpClient *http.Client
	apiURL     string
	model      string
}

// --- OpenAI-compatible request/response types ---

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatTemplateKwargs struct {
	Thinking bool `json:"thinking"`
}

type extraBody struct {
	ChatTemplateKwargs chatTemplateKwargs `json:"chat_template_kwargs"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	TopP        float64         `json:"top_p"`
	MaxTokens   int             `json:"max_tokens"`
	Stream      bool            `json:"stream"`
	ExtraBody   extraBody       `json:"extra_body"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewClaudeClient creates an LLM client pointed at NVIDIA's inference API.
// Falls back to CLAUDE_API_KEY / CLAUDE_MODEL env vars for backward compat.
func NewClaudeClient() *ClaudeClient {
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("CLAUDE_API_KEY") // legacy fallback
	}

	model := os.Getenv("NVIDIA_MODEL")
	if model == "" {
		model = "deepseek-ai/deepseek-v4-pro"
	}

	apiURL := os.Getenv("NVIDIA_API_URL")
	if apiURL == "" {
		apiURL = "https://integrate.api.nvidia.com/v1/chat/completions"
	}

	return &ClaudeClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		apiURL: apiURL,
		model:  model,
	}
}

// Complete sends a prompt to the LLM and returns the response text.
// The system prompt is prepended as a "system" role message.
func (c *ClaudeClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("NVIDIA_API_KEY not set")
	}

	messages := []openAIMessage{}
	if systemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: userPrompt})

	reqBody := openAIRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 1.0,
		TopP:        0.95,
		MaxTokens:   16384,
		Stream:      false, // we read the full response, not a stream
		ExtraBody: extraBody{
			ChatTemplateKwargs: chatTemplateKwargs{Thinking: false},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if oaiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return oaiResp.Choices[0].Message.Content, nil
}
