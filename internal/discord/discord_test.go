package discord_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"snow-white/internal/discord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSend_PostsJSONAndSucceedsOn204: the client POSTs {"content":"hello"}
// to the webhook URL and returns nil on a 204 response.
func TestSend_PostsJSONAndSucceedsOn204(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = body
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer srv.Close()

	c := discord.New(srv.URL)
	err := c.Send(context.Background(), "hello")
	require.NoError(t, err)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "hello", payload["content"])
}

// TestSend_EmptyURL_NoOp: New("").Send makes no HTTP call and returns nil.
func TestSend_EmptyURL_NoOp(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// Use empty URL — must NOT call the test server.
	c := discord.New("")
	err := c.Send(context.Background(), "anything")
	require.NoError(t, err)
	assert.False(t, called, "no HTTP call expected when URL is empty")
}

// TestSend_NonOK_ReturnsError: a 400 response causes Send to return a non-nil error.
func TestSend_NonOK_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400
	}))
	defer srv.Close()

	c := discord.New(srv.URL)
	err := c.Send(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
