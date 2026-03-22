package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to extract port from httptest.Server URL
func testServerPort(t *testing.T, s *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return port
}

func TestChatCompletion_Success(t *testing.T) {
	resp := ChatCompletionResponse{
		Choices: []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{Role: "assistant", Content: "Hello, world!"},
				FinishReason: "stop",
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req ChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.False(t, req.Stream)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	content, err := client.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", content)
}

func TestChatCompletion_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	_, err := client.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestChatCompletion_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{})
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	_, err := client.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestChatCompletion_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{invalid json"))
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	_, err := client.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})

	require.Error(t, err)
}

func TestChatCompletion_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled - the request should be cancelled before response
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := NewClient(testServerPort(t, ts))
	_, err := client.ChatCompletion(ctx, []Message{
		{Role: "user", Content: "Hi"},
	})

	require.Error(t, err)
}

// --- Streaming SSE tests ---

func makeSSEChunk(content string) string {
	chunk := ChatCompletionChunk{
		Choices: []struct {
			Delta struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				}{Content: content},
			},
		},
	}
	data, _ := json.Marshal(chunk)
	return fmt.Sprintf("data: %s\n\n", data)
}

func makeSSEChunkEmpty() string {
	chunk := ChatCompletionChunk{
		Choices: []struct {
			Delta struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				}{},
			},
		},
	}
	data, _ := json.Marshal(chunk)
	return fmt.Sprintf("data: %s\n\n", data)
}

func TestStreamChatCompletion_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req ChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.True(t, req.Stream)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		fmt.Fprint(w, makeSSEChunk("Hello"))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk(", "))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk("world!"))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var tokens []string
	var mu sync.Mutex
	result, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		mu.Lock()
		tokens = append(tokens, token)
		mu.Unlock()
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Hello", ", ", "world!"}, tokens)
	// No usage in this response
	assert.Equal(t, 0, result.Usage.PromptTokens)
}

func TestStreamChatCompletion_EmptyDeltas(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		fmt.Fprint(w, makeSSEChunk("Hi"))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunkEmpty()) // empty delta - should be skipped
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk("!"))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var tokens []string
	_, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		tokens = append(tokens, token)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Hi", "!"}, tokens)
}

func TestStreamChatCompletion_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var called bool
	_, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		called = true
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.False(t, called, "onToken should not be called on HTTP error")
}

func TestStreamChatCompletion_SkipsNonDataLines(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// SSE comment and empty lines should be skipped
		fmt.Fprint(w, ": this is a comment\n\n")
		flusher.Flush()
		fmt.Fprint(w, "\n")
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk("token"))
		flusher.Flush()
		fmt.Fprint(w, "event: ping\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var tokens []string
	_, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		tokens = append(tokens, token)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"token"}, tokens)
}

func TestStreamChatCompletion_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Block until context is cancelled
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := NewClient(testServerPort(t, ts))
	_, err := client.StreamChatCompletion(ctx, []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {})

	require.Error(t, err)
}

// makeSSEChunkWithUsage creates an SSE chunk that includes usage data (like the final chunk from llama.cpp).
func makeSSEChunkWithUsage(content string, promptTokens, completionTokens, totalTokens int) string {
	finish := "stop"
	chunk := struct {
		Choices []struct {
			Delta struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage,omitempty"`
	}{
		Choices: []struct {
			Delta struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				}{Content: content},
				FinishReason: &finish,
			},
		},
		Usage: &Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:       totalTokens,
		},
	}
	data, _ := json.Marshal(chunk)
	return fmt.Sprintf("data: %s\n\n", data)
}

func TestStreamChatCompletion_ReturnsUsage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		fmt.Fprint(w, makeSSEChunk("Hello"))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk(" world"))
		flusher.Flush()
		// Final chunk with usage data
		fmt.Fprint(w, makeSSEChunkWithUsage("!", 10, 5, 15))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var tokens []string
	result, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		tokens = append(tokens, token)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Hello", " world", "!"}, tokens)
	assert.Equal(t, 10, result.Usage.PromptTokens)
	assert.Equal(t, 5, result.Usage.CompletionTokens)
	assert.Equal(t, 15, result.Usage.TotalTokens)
}

func TestStreamChatCompletion_ZeroUsageWhenMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		fmt.Fprint(w, makeSSEChunk("Hi"))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	result, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {})

	require.NoError(t, err)
	assert.Equal(t, 0, result.Usage.PromptTokens)
	assert.Equal(t, 0, result.Usage.CompletionTokens)
	assert.Equal(t, 0, result.Usage.TotalTokens)
}

func TestStreamChatCompletion_OnTokenStillReceivesAllDeltas(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		fmt.Fprint(w, makeSSEChunk("a"))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunk("b"))
		flusher.Flush()
		fmt.Fprint(w, makeSSEChunkWithUsage("c", 20, 10, 30))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewClient(testServerPort(t, ts))
	var tokens []string
	result, err := client.StreamChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(token string) {
		tokens = append(tokens, token)
	})

	require.NoError(t, err)
	// All content deltas are received
	assert.Equal(t, []string{"a", "b", "c"}, tokens)
	// Usage is also captured
	assert.Equal(t, 20, result.Usage.PromptTokens)
	assert.Equal(t, 10, result.Usage.CompletionTokens)
}
