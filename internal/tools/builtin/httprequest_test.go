package builtin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRequest_get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(200)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tool := HTTPRequest{}
	input, _ := json.Marshal(map[string]any{"url": srv.URL})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "status: 200") || !strings.Contains(result, "hello") {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestHTTPRequest_postWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(201)
		w.Write(body) // echo back
	}))
	defer srv.Close()

	tool := HTTPRequest{}
	input, _ := json.Marshal(map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   `{"key":"val"}`,
		"headers": map[string]string{
			"Content-Type": "application/json",
		},
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "status: 201") || !strings.Contains(result, `{"key":"val"}`) {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestHTTPRequest_4xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	tool := HTTPRequest{}
	input, _ := json.Marshal(map[string]any{"url": srv.URL})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestHTTPRequest_5xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	tool := HTTPRequest{}
	input, _ := json.Marshal(map[string]any{"url": srv.URL})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

func TestHTTPRequest_missingURL(t *testing.T) {
	tool := HTTPRequest{}
	input, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("expected 'url is required' error, got: %v", err)
	}
}
