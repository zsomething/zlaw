package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
)

// WebSearch performs a web search and returns a list of results.
// Backend is selected by environment variables:
//
//	BRAVE_SEARCH_API_KEY  → Brave Search API
//	SEARXNG_BASE_URL      → SearXNG instance
type WebSearch struct{}

var webSearchSchema = []byte(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Search query string."
    },
    "num_results": {
      "type": "integer",
      "description": "Number of results to return. Defaults to 5.",
      "minimum": 1,
      "maximum": 20
    }
  },
  "required": ["query"]
}`)

type webSearchInput struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (WebSearch) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web and return a list of results with title, URL, and snippet. Requires BRAVE_SEARCH_API_KEY or SEARXNG_BASE_URL to be set.",
		InputSchema: webSearchSchema,
	}
}

func (WebSearch) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var input webSearchInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("web_search: invalid input: %w", err)
	}
	if input.Query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	n := input.NumResults
	if n <= 0 {
		n = 5
	}
	if n > 20 {
		n = 20
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var results []searchResult
	var err error

	switch {
	case os.Getenv("BRAVE_SEARCH_API_KEY") != "":
		results, err = braveSearch(ctx, input.Query, n)
	case os.Getenv("SEARXNG_BASE_URL") != "":
		results, err = searxngSearch(ctx, input.Query, n)
	default:
		return "", fmt.Errorf("web_search: no backend configured — set BRAVE_SEARCH_API_KEY or SEARXNG_BASE_URL")
	}
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}

	return formatSearchResults(results), nil
}

func braveSearch(ctx context.Context, query string, n int) ([]searchResult, error) {
	apiKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	u := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query) + "&count=" + strconv.Itoa(n)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("brave: decode response: %w", err)
	}

	out := make([]searchResult, 0, len(payload.Web.Results))
	for _, r := range payload.Web.Results {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

func searxngSearch(ctx context.Context, query string, n int) ([]searchResult, error) {
	base := strings.TrimRight(os.Getenv("SEARXNG_BASE_URL"), "/")
	u := base + "/search?q=" + url.QueryEscape(query) + "&format=json&categories=general"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("searxng: decode response: %w", err)
	}

	limit := n
	if limit > len(payload.Results) {
		limit = len(payload.Results)
	}
	out := make([]searchResult, 0, limit)
	for _, r := range payload.Results[:limit] {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}

func formatSearchResults(results []searchResult) string {
	if len(results) == 0 {
		return "No results found."
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n   URL: %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
		if i < len(results)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
