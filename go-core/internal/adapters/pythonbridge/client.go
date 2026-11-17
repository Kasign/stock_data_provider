package pythonbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
	Client  *http.Client
}
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"request_id,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("python bridge returned HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, errors.New("PYTHON_BRIDGE_BASE_URL is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("python bridge URL must be an absolute HTTP(S) URL without embedded credentials, query or fragment")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("PYTHON_BRIDGE_TOKEN is required")
	}
	hc := cfg.Client
	if hc == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 20 * time.Minute
		}
		// Internal bridge credentials must never be forwarded to a redirect
		// target. Treat redirects as a response and let normal HTTP error mapping
		// surface the unexpected status to the workflow.
		hc = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Client{baseURL: base, token: cfg.Token, httpClient: hc}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode bridge request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build bridge request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Python-Bridge-Token", c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call python bridge %s: %w", path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read python bridge response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return fmt.Errorf("python bridge response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		_ = json.Unmarshal(payload, &apiErr)
		if apiErr.Code == "" || apiErr.Message == "" {
			var envelope struct {
				Detail struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"detail"`
			}
			if json.Unmarshal(payload, &envelope) == nil {
				if apiErr.Code == "" {
					apiErr.Code = envelope.Detail.Code
				}
				if apiErr.Message == "" {
					apiErr.Message = envelope.Detail.Message
				}
			}
		}
		apiErr.StatusCode = resp.StatusCode
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(payload))
		}
		if apiErr.Code == "" {
			apiErr.Code = "bridge_http_error"
		}
		return &apiErr
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode python bridge response %s: %w", path, err)
	}
	return nil
}

func (c *Client) Post(ctx context.Context, path string, in, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, in, out)
}

func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) Health(ctx context.Context) error {
	var response struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if err := c.Get(ctx, "/healthz", &response); err != nil {
		return err
	}
	if response.Status != "ok" || response.Service != "dsa-python-bridge" {
		return fmt.Errorf("unexpected Python bridge health response: status=%q service=%q", response.Status, response.Service)
	}
	return nil
}
