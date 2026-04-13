package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zsomething/zlaw/internal/llm"
)

// HTTPRequest makes arbitrary HTTP requests and returns status, headers, and body.
type HTTPRequest struct{}

var httpRequestSchema = []byte(`{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "description": "Request URL."
    },
    "method": {
      "type": "string",
      "description": "HTTP method: GET, POST, PUT, PATCH, DELETE. Defaults to GET.",
      "enum": ["GET", "POST", "PUT", "PATCH", "DELETE"]
    },
    "headers": {
      "type": "object",
      "description": "Optional request headers as key-value pairs.",
      "additionalProperties": {"type": "string"}
    },
    "body": {
      "type": "string",
      "description": "Optional request body."
    },
    "timeout_sec": {
      "type": "integer",
      "description": "Request timeout in seconds. Defaults to 15.",
      "minimum": 1,
      "maximum": 60
    }
  },
  "required": ["url"]
}`)

type httpRequestInput struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	TimeoutSec int               `json:"timeout_sec"`
}

func (HTTPRequest) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "http_request",
		Description: "Make an HTTP request (GET/POST/PUT/PATCH/DELETE) and return the status code, response headers, and body. 4xx/5xx responses are returned as errors with the body included.",
		InputSchema: httpRequestSchema,
	}
}

func (HTTPRequest) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var input httpRequestInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("http_request: invalid input: %w", err)
	}
	if input.URL == "" {
		return "", fmt.Errorf("http_request: url is required")
	}

	method := strings.ToUpper(input.Method)
	if method == "" {
		method = http.MethodGet
	}

	timeout := input.TimeoutSec
	if timeout <= 0 {
		timeout = 15
	}
	if timeout > 60 {
		timeout = 60
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if input.Body != "" {
		bodyReader = bytes.NewBufferString(input.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, input.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("http_request: %w", err)
	}
	for k, v := range input.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "zlaw-agent/1.0")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http_request: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", fmt.Errorf("http_request: reading body: %w", err)
	}

	result := formatHTTPResponse(resp, respBody)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http_request: %s %s returned %d:\n%s", method, input.URL, resp.StatusCode, result)
	}
	return result, nil
}

func formatHTTPResponse(resp *http.Response, body []byte) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	fmt.Fprintf(&sb, "headers:\n")
	for k, vals := range resp.Header {
		fmt.Fprintf(&sb, "  %s: %s\n", k, strings.Join(vals, ", "))
	}
	if len(body) > 0 {
		fmt.Fprintf(&sb, "body:\n%s", string(body))
		if len(body) == 0 || body[len(body)-1] != '\n' {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
