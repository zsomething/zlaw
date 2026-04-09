package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearch_noBackend(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	t.Setenv("SEARXNG_BASE_URL", "")
	tool := WebSearch{}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when no backend configured")
	}
	if !strings.Contains(err.Error(), "no backend configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWebSearch_missingQuery(t *testing.T) {
	tool := WebSearch{}
	input, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Errorf("expected 'query is required' error, got: %v", err)
	}
}

func TestWebSearch_searxng(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Result One", "url": "https://example.com/1", "content": "First snippet"},
				{"title": "Result Two", "url": "https://example.com/2", "content": "Second snippet"},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	t.Setenv("SEARXNG_BASE_URL", srv.URL)

	tool := WebSearch{}
	input, _ := json.Marshal(map[string]any{"query": "golang", "num_results": 2})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Result One") || !strings.Contains(result, "First snippet") {
		t.Errorf("expected result content, got: %q", result)
	}
	if !strings.Contains(result, "https://example.com/1") {
		t.Errorf("expected URL in result, got: %q", result)
	}
}

func TestWebSearch_brave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Brave Result", "url": "https://brave.com/1", "description": "Brave snippet"},
				},
			},
		})
	}))
	defer srv.Close()

	// Point brave at our test server by overriding; we test brave parsing via a
	// mock that mimics the response shape without hitting the real API.
	// Since braveSearch hardcodes the URL, we test the format helpers directly.
	_ = srv

	results := []searchResult{
		{Title: "Brave Result", URL: "https://brave.com/1", Snippet: "Brave snippet"},
	}
	out := formatSearchResults(results)
	if !strings.Contains(out, "Brave Result") || !strings.Contains(out, "Brave snippet") {
		t.Errorf("formatSearchResults output unexpected: %q", out)
	}
}

func TestWebSearch_formatEmpty(t *testing.T) {
	out := formatSearchResults(nil)
	if out != "No results found." {
		t.Errorf("expected 'No results found.', got: %q", out)
	}
}
