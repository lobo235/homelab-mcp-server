package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	anthropicModel      = "claude-sonnet-4-6"
	webSearchTimeout    = 120 * time.Second

	// webSearchMinInterval is the minimum time between web search API calls.
	// The Anthropic API has a 30K input tokens/minute rate limit, and each
	// web search call uses ~8-15K tokens. 90 seconds prevents rate limiting.
	webSearchMinInterval = 90 * time.Second
)

// webSearchLimiter enforces minimum interval between web search API calls.
var webSearchLimiter = struct {
	mu       sync.Mutex
	lastCall time.Time
}{}

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
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason,omitempty"`
	Error      *anthropicError         `json:"error,omitempty"`
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

// rateLimitError is returned when the API responds with 429.
type rateLimitError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("api returned status 429 (retry after %s): %s", e.RetryAfter, e.Body)
}

// waitForRateLimit blocks until the minimum interval between web search calls has elapsed.
func waitForRateLimit(ctx context.Context) error {
	webSearchLimiter.mu.Lock()
	if !webSearchLimiter.lastCall.IsZero() {
		elapsed := time.Since(webSearchLimiter.lastCall)
		if wait := webSearchMinInterval - elapsed; wait > 0 {
			webSearchLimiter.mu.Unlock()
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
			webSearchLimiter.mu.Lock()
		}
	}
	webSearchLimiter.lastCall = time.Now()
	webSearchLimiter.mu.Unlock()
	return nil
}

// callWebSearch calls the Anthropic API with web search capability and returns
// the text content from the assistant's response. Enforces a minimum interval
// between calls to stay within API rate limits.
func callWebSearch(ctx context.Context, apiKey, systemPrompt, userPrompt string, maxSearchUses int) (string, error) {
	if err := waitForRateLimit(ctx); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()

	reqBody := anthropicRequest{
		Model:     anthropicModel,
		MaxTokens: 16384,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
		Tools: []anthropicTool{
			{Type: "web_search_20250305", Name: "web_search", MaxUses: maxSearchUses},
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

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 30 * time.Second // default fallback
		if ra := resp.Header.Get("retry-after"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return "", &rateLimitError{RetryAfter: retryAfter, Body: string(respBody)}
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
	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" && block.Text != "" {
			text = block.Text
			break
		}
	}

	// Detect max_tokens truncation — can happen with or without text content.
	if apiResp.StopReason == "max_tokens" {
		return "", fmt.Errorf("response truncated (max_tokens reached, got %d text chars)", len(text))
	}

	if text == "" {
		return "", fmt.Errorf("no text content in response (stop_reason=%s, blocks=%d)", apiResp.StopReason, len(apiResp.Content))
	}

	// Short responses without JSON are likely truncated even if stop_reason is end_turn.
	if len(text) < 200 && !strings.Contains(text, "{") {
		return "", fmt.Errorf("response too short and missing JSON (max_tokens likely insufficient, got %d chars, stop_reason=%s)", len(text), apiResp.StopReason)
	}

	return text, nil
}

// callWebSearchWithRetry wraps callWebSearch with retry logic for transient errors.
// It retries up to 2 times (3 total attempts) with exponential backoff.
// Auth errors (401/403) are NOT retried. On max_tokens truncation, retries with
// fewer web search uses to leave more token budget for the response.
func callWebSearchWithRetry(ctx context.Context, apiKey, systemPrompt, userPrompt string) (string, error) {
	const maxAttempts = 3

	// Start with 5 web searches; reduce on truncation to leave room for the response.
	maxUses := 5

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		result, err := callWebSearch(ctx, apiKey, systemPrompt, userPrompt, maxUses)
		if err == nil {
			return result, nil
		}

		lastErr = err
		errMsg := err.Error()

		// Don't retry auth errors — they won't succeed on retry.
		if strings.Contains(errMsg, "status 401") || strings.Contains(errMsg, "status 403") {
			return "", fmt.Errorf("authentication error (check ANTHROPIC_API_KEY): %w", err)
		}

		// Don't retry 400 errors (bad request).
		if strings.Contains(errMsg, "status 400") {
			return "", err
		}

		// Rate limit errors: use retry-after header from the API.
		var rlErr *rateLimitError
		if errors.As(err, &rlErr) {
			select {
			case <-time.After(rlErr.RetryAfter):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}

		// Truncation: reduce web search uses to leave more token budget for the response.
		if strings.Contains(errMsg, "max_tokens") {
			maxUses = 2
			continue
		}

		// Retry all other errors (5xx, timeouts, connection errors).
	}

	return "", fmt.Errorf("web search failed after %d attempts: %w", maxAttempts, lastErr)
}
