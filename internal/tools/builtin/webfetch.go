package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/chickenzord/zlaw/internal/llm"
)

// WebFetch fetches a URL and returns its content as readable text.
type WebFetch struct{}

var webFetchSchema = []byte(`{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "description": "The URL to fetch."
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

type webFetchInput struct {
	URL        string `json:"url"`
	TimeoutSec int    `json:"timeout_sec"`
}

func (WebFetch) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "web_fetch",
		Description: "Fetch a URL and return its content. HTML responses are stripped to readable text; other content types are returned as-is.",
		InputSchema: webFetchSchema,
	}
}

func (WebFetch) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var input webFetchInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("web_fetch: invalid input: %w", err)
	}
	if input.URL == "" {
		return "", fmt.Errorf("web_fetch: url is required")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	req.Header.Set("User-Agent", "zlaw-agent/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_fetch: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", fmt.Errorf("web_fetch: reading body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	var content string
	if strings.Contains(contentType, "text/html") {
		content = htmlToText(string(body))
	} else {
		content = string(body)
	}

	return fmt.Sprintf("url: %s\nstatus: %d\n\n%s", input.URL, resp.StatusCode, content), nil
}

// htmlToText extracts visible text from HTML using the tokenizer.
func htmlToText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var sb strings.Builder
	skip := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(sb.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch tag {
			case "script", "style", "head", "noscript", "template":
				skip = true
			case "br", "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr":
				if !skip {
					sb.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch tag {
			case "script", "style", "head", "noscript", "template":
				skip = false
			}
		case html.TextToken:
			if !skip {
				text := strings.TrimSpace(string(z.Text()))
				if text != "" {
					sb.WriteString(text)
					sb.WriteByte(' ')
				}
			}
		}
	}
}
