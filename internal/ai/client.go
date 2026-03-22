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
	EndpointURL = "https://marketriskmonitor.com/api/analyze"
	ModelID     = "claude-opus-4-6"
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
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	System   string         `json:"system,omitempty"`
	Thinking ThinkingConfig `json:"thinking,omitempty"`
	MaxTokens int           `json:"max_tokens"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Response struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"anthropic_error,omitempty"`
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) Chat(system string, messages []Message, maxTokens int) (string, error) {
	req := Request{
		Model:     ModelID,
		Messages:  messages,
		System:    system,
		MaxTokens: maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", EndpointURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var aiResp Response
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("JSON parse error: %w", err)
	}

	if aiResp.Error != nil {
		return "", fmt.Errorf("AI error: %s", aiResp.Error.Message)
	}

	for _, block := range aiResp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

func (c *Client) ChatWithThinking(system string, messages []Message, maxTokens, thinkingBudget int) (string, error) {
	req := Request{
		Model:    ModelID,
		Messages: messages,
		System:   system,
		Thinking: ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: thinkingBudget,
		},
		MaxTokens: maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", EndpointURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var aiResp Response
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("JSON parse error: %w", err)
	}

	if aiResp.Error != nil {
		return "", fmt.Errorf("AI error: %s", aiResp.Error.Message)
	}

	// Return only text blocks (skip thinking blocks)
	var text string
	for _, block := range aiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if text == "" {
		return "", fmt.Errorf("no text in response")
	}
	return text, nil
}
