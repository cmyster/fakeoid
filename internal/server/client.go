package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client communicates with a running llama-server instance.
type Client struct {
	port          int
	httpClient    *http.Client
	sseBufferSize int
}

// NewClient creates a new Client for the given port.
// chatTimeoutSec sets the HTTP timeout for non-streaming requests (0 = 10s default).
// sseBufferSize sets the max SSE line buffer in bytes (0 = 1MB default).
func NewClient(port int, chatTimeoutSec int, sseBufferSize int) *Client {
	if chatTimeoutSec == 0 {
		chatTimeoutSec = 10
	}
	if sseBufferSize == 0 {
		sseBufferSize = 1024 * 1024
	}
	return &Client{
		port:          port,
		httpClient:    &http.Client{Timeout: time.Duration(chatTimeoutSec) * time.Second},
		sseBufferSize: sseBufferSize,
	}
}

// ChatCompletion sends a non-streaming chat completion request.
func (c *Client) ChatCompletion(ctx context.Context, messages []Message) (string, error) {
	body := ChatRequest{
		Messages: messages,
		Stream:   false,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", c.port),
		bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat completion request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat completion failed with status %d", resp.StatusCode)
	}

	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// StreamChatCompletion sends a streaming chat completion request and calls onToken
// for each content delta received via SSE. Returns a StreamResult with token usage
// data parsed from the final SSE chunk.
func (c *Client) StreamChatCompletion(ctx context.Context, messages []Message, onToken func(string)) (StreamResult, error) {
	var result StreamResult

	body := ChatRequest{
		Messages: messages,
		Stream:   true,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return result, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", c.port),
		bytes.NewReader(jsonBody))
	if err != nil {
		return result, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a client without timeout for streaming (timeout applies to entire response)
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return result, fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("stream request failed with status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, c.sseBufferSize), c.sseBufferSize)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onToken(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil {
			result.Usage = *chunk.Usage
		}
	}
	return result, scanner.Err()
}
