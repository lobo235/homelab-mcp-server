package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	anthropicModel      = "claude-sonnet-4-5-20241022"
	webSearchTimeout    = 60 * time.Second
)

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools"`
}

// anthropicMessage is a single message in the conversation.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicTool defines a tool available to the model.
type anthropicTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses,omitempty"`
}

// anthropicResponse is the response from the Anthropic Messages API.
type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicError         `json:"error,omitempty"`
}

// anthropicContentBlock is a single content block in the response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// anthropicError represents an API error.
type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// callWebSearch calls the Anthropic API with web search capability and returns
// the text content from the assistant's response.
func callWebSearch(ctx context.Context, apiKey, systemPrompt, userPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()

	reqBody := anthropicRequest{
		Model:     anthropicModel,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
		Tools: []anthropicTool{
			{Type: "web_search_20250305", Name: "web_search", MaxUses: 5},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to prevent resource exhaustion (50 MB).
	const maxResponseSize = 50 * 1024 * 1024
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("api error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	// Extract text content from the response.
	for _, block := range apiResp.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}
