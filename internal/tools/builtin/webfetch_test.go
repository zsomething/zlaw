package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetch_htmlToText(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantText string
	}{
		{
			name:     "basic paragraph",
			html:     "<html><body><p>Hello world</p></body></html>",
			wantText: "Hello world",
		},
		{
			name:     "strips script tags",
			html:     "<html><head><script>alert('x')</script></head><body><p>Visible</p></body></html>",
			wantText: "Visible",
		},
		{
			name:     "strips style tags",
			html:     "<html><head><style>body{color:red}</style></head><body><p>Text</p></body></html>",
			wantText: "Text",
		},
		{
			name:     "multiple elements",
			html:     "<html><body><h1>Title</h1><p>Para</p></body></html>",
			wantText: "Title",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToText(tc.html)
			if !strings.Contains(got, tc.wantText) {
				t.Errorf("htmlToText(%q) = %q, want it to contain %q", tc.html, got, tc.wantText)
			}
			// Should never contain script/style content
			if strings.Contains(got, "alert") || strings.Contains(got, "color:red") {
				t.Errorf("htmlToText leaked script/style content: %q", got)
			}
		})
	}
}

func TestWebFetch_Execute(t *testing.T) {
	t.Run("fetches HTML and strips tags", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(200)
			w.Write([]byte("<html><body><p>Hello from server</p></body></html>"))
		}))
		defer srv.Close()

		tool := WebFetch{}
		input, _ := json.Marshal(map[string]any{"url": srv.URL})
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "Hello from server") {
			t.Errorf("expected text content in result, got: %q", result)
		}
		if !strings.Contains(result, "status: 200") {
			t.Errorf("expected status line in result, got: %q", result)
		}
	})

	t.Run("returns raw body for non-HTML", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"key":"value"}`))
		}))
		defer srv.Close()

		tool := WebFetch{}
		input, _ := json.Marshal(map[string]any{"url": srv.URL})
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, `{"key":"value"}`) {
			t.Errorf("expected raw JSON in result, got: %q", result)
		}
	})

	t.Run("error on missing url", func(t *testing.T) {
		tool := WebFetch{}
		input, _ := json.Marshal(map[string]any{})
		_, err := tool.Execute(context.Background(), input)
		if err == nil {
			t.Fatal("expected error for missing url")
		}
	})

	t.Run("error on invalid url", func(t *testing.T) {
		tool := WebFetch{}
		input, _ := json.Marshal(map[string]any{"url": "not-a-url"})
		_, err := tool.Execute(context.Background(), input)
		if err == nil {
			t.Fatal("expected error for invalid url")
		}
	})
}
