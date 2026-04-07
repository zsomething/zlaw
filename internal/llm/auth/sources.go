package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// staticKeySource returns a fixed API key.
type staticKeySource struct {
	key string
}

func (s *staticKeySource) Token(_ context.Context) (string, error) {
	return s.key, nil
}

// oauth2Source performs the client_credentials grant and caches the token.
// It refreshes automatically when within refreshWindow of expiry.
type oauth2Source struct {
	profile CredentialProfile
	mu      *sync.Mutex

	cachedToken  string
	cachedExpiry time.Time
}

const refreshWindow = 60 * time.Second

func (s *oauth2Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return cached token if still fresh.
	if s.cachedToken != "" && time.Until(s.cachedExpiry) > refreshWindow {
		return s.cachedToken, nil
	}

	// Use stored access token from credentials file if not expired.
	if s.profile.AccessToken != "" && !s.profile.Expiry.IsZero() && time.Until(s.profile.Expiry) > refreshWindow {
		s.cachedToken = s.profile.AccessToken
		s.cachedExpiry = s.profile.Expiry
		return s.cachedToken, nil
	}

	// Need to fetch a new token via client_credentials.
	if s.profile.TokenURL == "" || s.profile.ClientID == "" || s.profile.ClientSecret == "" {
		return "", fmt.Errorf("auth: oauth2 profile missing token_url, client_id, or client_secret — run 'zlaw-agent auth login' to configure")
	}

	token, expiry, err := fetchClientCredentials(ctx, s.profile)
	if err != nil {
		return "", err
	}
	s.cachedToken = token
	s.cachedExpiry = expiry
	return token, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // seconds
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func fetchClientCredentials(ctx context.Context, p CredentialProfile) (string, time.Time, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
	}
	if p.Scope != "" {
		data.Set("scope", p.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: parse token response: %w", err)
	}
	if tr.Error != "" {
		return "", time.Time{}, fmt.Errorf("auth: token error %q: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("auth: empty access_token in response")
	}

	expiry := time.Time{}
	if tr.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tr.AccessToken, expiry, nil
}
