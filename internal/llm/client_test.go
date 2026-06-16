package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientChat(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "Hello! How can I help?",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	reply, err := client.Chat(t.Context(), "You are helpful", "Hi")
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if reply != "Hello! How can I help?" {
		t.Errorf("Chat() = %q, want %q", reply, "Hello! How can I help?")
	}
}

func TestClientChatJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: `{"name": "Alice", "age": 30}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	err := client.ChatJSON(t.Context(), "Extract info", "Alice is 30", &result)
	if err != nil {
		t.Fatalf("ChatJSON() error: %v", err)
	}
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("ChatJSON() = %+v, want {Alice 30}", result)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "extracts from code fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "extracts raw JSON",
			input: `some text {"a": 1} more text`,
			want:  `{"a": 1}`,
		},
		{
			name:  "extracts nested JSON",
			input: `{"outer": {"inner": [1, 2, 3]}}`,
			want:  `{"outer": {"inner": [1, 2, 3]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "bad-key",
		Model:   "test-model",
	})

	_, err := client.Chat(t.Context(), "prompt", "hello")
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestChatJSONEscapedQuotes(t *testing.T) {
	// Simulate LLM returning JSON with escaped quotes (common issue)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: `[{\"type\":\"dialogue\",\"role_name\":\"旁白\",\"text_content\":\"测试文本\"}]`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	var result []struct {
		Type        string `json:"type"`
		RoleName    string `json:"role_name"`
		TextContent string `json:"text_content"`
	}
	err := client.ChatJSON(t.Context(), "Analyze chapter", "Chapter text", &result)
	if err != nil {
		t.Fatalf("ChatJSON() with escaped quotes error: %v", err)
	}
	if len(result) != 1 || result[0].RoleName != "旁白" {
		t.Errorf("ChatJSON() = %+v, want 1 item with role_name=旁白", result)
	}
}

func TestFixLLMJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "removes trailing comma before ]",
			input: `[{"a": 1,}]`,
			want:  `[{"a": 1}]`,
		},
		{
			name:  "removes trailing comma before }",
			input: `{"a": 1,}`,
			want:  `{"a": 1}`,
		},
		{
			name:  "fixes unclosed string",
			input: `{"a": "hello`,
			want:  `{"a": "hello"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixLLMJSON(tt.input)
			if got != tt.want {
				t.Errorf("fixLLMJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectSSEContent(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "single chunk with delta.content",
			body: `data: {"choices":[{"delta":{"content":"Hello world"}}]}` + "\n\n",
			want: "Hello world",
		},
		{
			name: "multiple chunks accumulate",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
				"data: [DONE]\n\n",
			want: "Hello world",
		},
		{
			name: "handles role announcement chunk",
			body: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"Hi!\"}}]}\n\n" +
				"data: [DONE]\n\n",
			want: "Hi!",
		},
		{
			name: "non-SSE body returns empty",
			body: `{"choices":[{"message":{"content":"Hello"}}]}`,
			want: "", // Not SSE format, so collectSSEContent returns empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectSSEContent(tt.body)
			if got != tt.want {
				t.Errorf("collectSSEContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientChat_SSEMode(t *testing.T) {
	// Simulate an LLM router that returns SSE stream even for non-streaming requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return SSE-formatted data (like many proxy/router do)
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"The capital of France is Paris.\"}}]}\n\n" +
			"data: [DONE]\n\n"
		w.Write([]byte(body))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	reply, err := client.Chat(t.Context(), "You are helpful", "What is the capital of France?")
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if reply != "The capital of France is Paris." {
		t.Errorf("Chat() = %q, want %q", reply, "The capital of France is Paris.")
	}
}
