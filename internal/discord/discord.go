package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client sends messages to a Discord incoming webhook.
type Client struct {
	url string
	hc  *http.Client
}

// New creates a Client. If url is empty, Send is a no-op.
func New(url string) *Client {
	return &Client{
		url: url,
		hc:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts content as a Discord webhook message.
// If the URL is empty, Send is a no-op and returns nil.
// Treats any 2xx response (including 204 No Content) as success.
// Returns a non-nil error for non-2xx responses or transport failures.
func (c *Client) Send(ctx context.Context, content string) error {
	if c.url == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		var urlErr *net.OpError
		if errors.As(err, &urlErr) {
			return fmt.Errorf("discord: http: %w", urlErr.Err)
		}
		return fmt.Errorf("discord: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}
	return nil
}
