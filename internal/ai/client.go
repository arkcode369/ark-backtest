package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	EndpointURL     = "https://marketriskmonitor.com/api/analyze"
	ModelID         = "claude-opus-4-6"
	defaultTimeout  = 90 * time.Second  // regular chat
	thinkingTimeout = 180 * time.Second // extended thinking needs more time
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type Request struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	System    string          `json:"system,omitempty"`
	Thinking  *ThinkingConfig `json:"thinking,omitempty"` // pointer: nil = omitted from JSON
	MaxTokens int             `json:"max_tokens"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Response struct {
	ID      string         `json:"id"`
	Content []ContentBlock `json:"content"`
	// Anthropic error is wrapped here by the proxy
	AnthropicError *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"anthropic_error,omitempty"`
}

type Client struct{}

func NewClient() *Client { return &Client{} }

// doRequest executes an API call with the given timeout
func (c *Client) doRequest(req Request, timeout time.Duration) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal error: %w", err)
	}

	httpReq, err := http.NewRequest("POST", EndpointURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request error: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: timeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	// Guard: non-JSON response (e.g. HTML error page, rate limit page, timeout HTML)
	if len(respBody) == 0 {
		return "", fmt.Errorf("empty response from API (HTTP %d)", resp.StatusCode)
	}
	if respBody[0] != '{' {
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return "", fmt.Errorf("API returned non-JSON (HTTP %d): %s", resp.StatusCode, preview)
	}

	var aiResp Response
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("JSON parse failed: %w", err)
	}

	if aiResp.AnthropicError != nil {
		return "", fmt.Errorf("API error [%s]: %s", aiResp.AnthropicError.Type, aiResp.AnthropicError.Message)
	}

	// Collect all text blocks (thinking blocks are skipped)
	var text string
	for _, block := range aiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if text == "" {
		return "", fmt.Errorf("no text content in API response (received %d content blocks)", len(aiResp.Content))
	}
	return text, nil
}

// Chat sends a regular request (no extended thinking)
func (c *Client) Chat(system string, messages []Message, maxTokens int) (string, error) {
	req := Request{
		Model:     ModelID,
		Messages:  messages,
		System:    system,
		MaxTokens: maxTokens,
		// Thinking is nil → omitted from JSON
	}
	return c.doRequest(req, defaultTimeout)
}

// ChatWithThinking sends a request with extended thinking enabled.
// Uses a longer HTTP timeout (180s) since thinking adds latency.
func (c *Client) ChatWithThinking(system string, messages []Message, maxTokens, thinkingBudget int) (string, error) {
	req := Request{
		Model:    ModelID,
		Messages: messages,
		System:   system,
		Thinking: &ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: thinkingBudget,
		},
		MaxTokens: maxTokens,
	}
	return c.doRequest(req, thinkingTimeout)
}
