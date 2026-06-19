package invx

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestExactCaseHeadersOnWire guards against a regression where the X-INVX-*
// headers are sent canonicalized (e.g. "X-Invx-Request-Uid"). InnovestX matches
// header names case-sensitively and rejects the canonical form with code 4008,
// so the request must carry the exact all-caps names verbatim on the wire.
//
// httptest cannot catch this — its handler reads r.Header, which canonicalizes
// keys. This test reads the raw request bytes off a TCP socket instead.
func TestExactCaseHeadersOnWire(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	reqCh := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		var sb strings.Builder
		buf := make([]byte, 1024)
		for {
			n, rerr := conn.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			// Stop once the full header block has arrived (the body is tiny and
			// follows the blank line).
			if strings.Contains(sb.String(), "\r\n\r\n") || rerr != nil {
				break
			}
		}
		reqCh <- sb.String()

		body := `{"code":"0000","message":"SUCCESS","data":[]}`
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	}()

	host := ln.Addr().String() // 127.0.0.1:PORT
	c := New("pubkey", "secretval", host, nil)
	c.baseURL = "http://" + host // plaintext HTTP/1.1 for the raw read

	_, err = c.Ticker(context.Background(), "BTCTHB")
	require.NoError(t, err)

	raw := <-reqCh

	// Exact-case header names must appear verbatim on the wire.
	require.Contains(t, raw, "X-INVX-APIKEY: pubkey")
	require.Contains(t, raw, "X-INVX-REQUEST-UID: ")
	require.Contains(t, raw, "X-INVX-SIGNATURE: ")
	require.Contains(t, raw, "X-INVX-TIMESTAMP: ")

	// The canonicalized form (what Header.Set would produce) must NOT appear —
	// this is the actual regression guard.
	require.NotContains(t, raw, "X-Invx-Request-Uid")
	require.NotContains(t, raw, "X-Invx-Apikey")
}

func TestDecimalNumber(t *testing.T) {
	// 7000 satang -> "70.00" ; 10000000 (x1e8) -> "0.10000000"
	if got := decimalNumber(7000, 2); string(got) != "70.00" {
		t.Fatalf("price = %q, want 70.00", got)
	}
	if got := decimalNumber(10000000, 8); string(got) != "0.10000000" {
		t.Fatalf("qty = %q, want 0.10000000", got)
	}
	// Marshals as a bare JSON number, not a quoted string.
	b, err := json.Marshal(map[string]json.Number{"limitPrice": decimalNumber(7000, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"limitPrice":70.00}` {
		t.Fatalf("marshal = %s", b)
	}
}
