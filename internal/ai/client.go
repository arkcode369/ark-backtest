package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	EndpointURL     = "https://marketriskmonitor.com/api/analyze"
	ModelID         = "claude-opus-4-6"
	defaultTimeout  = 90 * time.Second  // regular chat
	thinkingTimeout = 180 * time.Second // extended thinking needs more time
	minThinkingBudget = 1024            // API minimum for budget_tokens
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
	Thinking  *ThinkingConfig `json:"thinking,omitempty"` // nil = omitted from JSON
	MaxTokens int             `json:"max_tokens"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// rawResponse captures all possible response shapes from the proxy
type rawResponse struct {
	ID      string         `json:"id"`
	Content []ContentBlock `json:"content"`

	// Proxy wraps Anthropic errors here
	AnthropicError *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"anthropic_error,omitempty"`

	// Proxy itself may return plain "error" string (e.g. JSON parse proxy errors)
	ProxyError string `json:"error,omitempty"`

	// Some proxies wrap in "message" field
	ProxyMessage string `json:"message,omitempty"`
}

// Session manages per-chat conversation history with mutex protection
type Session struct {
	mu       sync.Mutex
	messages []Message
}

func NewSession() *Session { return &Session{} }

func (s *Session) Append(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

func (s *Session) Snapshot() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out
}

func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

// Client handles all API communication
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
		return "", fmt.Errorf("request failed (timeout=%v): %w", timeout, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Guard: empty response
	if len(respBody) == 0 {
		return "", fmt.Errorf("empty response from API (HTTP %d)", resp.StatusCode)
	}

	// Guard: non-JSON (HTML error page, gateway timeout, etc.)
	if respBody[0] != '{' {
		preview := string(respBody)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return "", fmt.Errorf("non-JSON response (HTTP %d): %s", resp.StatusCode, preview)
	}

	var r rawResponse
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", fmt.Errorf("JSON parse failed: %w", err)
	}

	// Handle ALL error formats from the proxy
	if r.AnthropicError != nil {
		return "", fmt.Errorf("[%s] %s", r.AnthropicError.Type, r.AnthropicError.Message)
	}
	if r.ProxyError != "" {
		return "", fmt.Errorf("proxy error: %s", r.ProxyError)
	}
	if r.ProxyMessage != "" && len(r.Content) == 0 {
		return "", fmt.Errorf("API message: %s", r.ProxyMessage)
	}

	// Collect text blocks (thinking blocks are intentionally skipped)
	var text string
	for _, block := range r.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	if text == "" {
		// Log full raw for debugging without crashing
		raw := string(respBody)
		if len(raw) > 300 {
			raw = raw[:300] + "..."
		}
		return "", fmt.Errorf("no text in response (HTTP %d, blocks=%d): %s",
			resp.StatusCode, len(r.Content), raw)
	}
	return text, nil
}

// Chat sends a regular request (no extended thinking)
func (c *Client) Chat(system string, messages []Message, maxTokens int) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("messages cannot be empty")
	}
	req := Request{
		Model:     ModelID,
		Messages:  messages,
		System:    system,
		MaxTokens: maxTokens,
	}
	return c.doRequest(req, defaultTimeout)
}

// ChatWithThinking sends a request with extended thinking.
// Falls back to regular Chat if thinking fails.
func (c *Client) ChatWithThinking(system string, messages []Message, maxTokens, thinkingBudget int) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("messages cannot be empty")
	}
	// Enforce API minimum budget
	if thinkingBudget < minThinkingBudget {
		thinkingBudget = minThinkingBudget
	}
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
