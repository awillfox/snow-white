package invx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"snow-white/pkg/scale"
)

const basePath = "/api/v1/digital-asset"

type Client struct {
	apikey  string
	secret  string
	host    string // lowercase hostname, e.g. api-dev.innovestxonline.com
	baseURL string // https://<host>
	hc      *http.Client
	now     func() time.Time
}

// New builds an InnovestX client. If hc is nil, a client pinned to HTTP/1.1 is
// created (required — see below). If hc is non-nil, the caller MUST ensure its
// transport uses HTTP/1.1; an h2-capable client lowercases header names on the
// wire and the case-sensitive API then rejects every request with code 4008.
func New(apikey, secret, host string, hc *http.Client) *Client {
	if hc == nil {
		// InnovestX matches header names case-sensitively and requires the
		// exact-case X-INVX-* headers. HTTP/2 forces all header field names to
		// lowercase on the wire, so pin HTTP/1.1 to preserve their casing.
		// NextProtos must be set so TLS ALPN only offers http/1.1 — otherwise
		// the server negotiates h2 and Go's h1 transport sees raw h2 frames.
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ForceAttemptHTTP2 = false
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.NextProtos = []string{"http/1.1"}
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		hc = &http.Client{Timeout: 15 * time.Second, Transport: tr}
	}
	host = strings.ToLower(host)
	return &Client{
		apikey:  apikey,
		secret:  secret,
		host:    host,
		baseURL: "https://" + host,
		hc:      hc,
		now:     time.Now,
	}
}

type apiResponse struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Data    []tickerRawDA `json:"data"`
}

type tickerRawDA struct {
	DateTime       string `json:"dateTime"`
	High           string `json:"high"`
	Low            string `json:"low"`
	Open           string `json:"open"`
	Close          string `json:"close"`
	Volume         string `json:"volume"`
	InsideBidPrice string `json:"insideBidPrice"`
	InsideAskPrice string `json:"insideAskPrice"`
	Symbol         string `json:"symbol"`
}

type TickerCandle struct {
	DateTime  time.Time
	Open      int64
	High      int64
	Low       int64
	Close     int64
	Volume    int64
	InsideBid int64
	InsideAsk int64
	Symbol    string
}

func (c *Client) Ticker(ctx context.Context, symbol string) ([]TickerCandle, error) {
	body, err := json.Marshal(map[string]string{"symbol": symbol})
	if err != nil {
		return nil, fmt.Errorf("marshal ticker body: %w", err)
	}
	raw, err := c.post(ctx, "/ticker/subscribe", body)
	if err != nil {
		return nil, err
	}
	var resp apiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode ticker resp: %w", err)
	}
	if resp.Code != "0000" {
		return nil, fmt.Errorf("ticker %s: api error %s: %s", symbol, resp.Code, resp.Message)
	}
	out := make([]TickerCandle, 0, len(resp.Data))
	for _, d := range resp.Data {
		tc, err := d.toCandle()
		if err != nil {
			return nil, fmt.Errorf("parse candle for %s: %w", symbol, err)
		}
		out = append(out, tc)
	}
	return out, nil
}

func (d tickerRawDA) toCandle() (TickerCandle, error) {
	dt, err := time.Parse(time.RFC3339, d.DateTime)
	if err != nil {
		return TickerCandle{}, fmt.Errorf("dateTime %q: %w", d.DateTime, err)
	}
	p := func(s string) (int64, error) { return scale.Parse(s, 2) }   // satang
	v := func(s string) (int64, error) { return scale.Parse(s, 8) }   // asset *1e8
	open, err := p(d.Open)
	if err != nil { return TickerCandle{}, err }
	high, err := p(d.High)
	if err != nil { return TickerCandle{}, err }
	low, err := p(d.Low)
	if err != nil { return TickerCandle{}, err }
	cls, err := p(d.Close)
	if err != nil { return TickerCandle{}, err }
	vol, err := v(d.Volume)
	if err != nil { return TickerCandle{}, err }
	bid, err := p(d.InsideBidPrice)
	if err != nil { return TickerCandle{}, err }
	ask, err := p(d.InsideAskPrice)
	if err != nil { return TickerCandle{}, err }
	return TickerCandle{
		DateTime: dt, Open: open, High: high, Low: low, Close: cls,
		Volume: vol, InsideBid: bid, InsideAsk: ask, Symbol: d.Symbol,
	}, nil
}

// post signs and sends a POST to basePath+path. body is the exact bytes signed and sent.
func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	fullPath := basePath + path
	uid := uuid.NewString()
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)

	sts := buildStringToSign(c.apikey, "POST", c.host, fullPath, "", contentType, uid, ts, string(body))
	signature := sign(c.secret, sts)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+fullPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// Assign directly to the header map (not Header.Set) so the exact-case
	// X-INVX-* names go on the wire verbatim. Header.Set canonicalizes them to
	// "X-Invx-..." which the case-sensitive API rejects with code 4008.
	req.Header["Content-Type"] = []string{contentType}
	req.Header["X-INVX-APIKEY"] = []string{c.apikey}
	req.Header["X-INVX-SIGNATURE"] = []string{signature}
	req.Header["X-INVX-REQUEST-UID"] = []string{uid}
	req.Header["X-INVX-TIMESTAMP"] = []string{ts}

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request %s: %w", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d for %s: %s", res.StatusCode, path, string(raw))
	}
	return raw, nil
}

// get signs and sends a GET to basePath+path (empty body in the signature).
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	fullPath := basePath + path
	uid := uuid.NewString()
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)

	sts := buildStringToSign(c.apikey, "GET", c.host, fullPath, "", contentType, uid, ts, "")
	signature := sign(c.secret, sts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+fullPath, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header["Content-Type"] = []string{contentType}
	req.Header["X-INVX-APIKEY"] = []string{c.apikey}
	req.Header["X-INVX-SIGNATURE"] = []string{signature}
	req.Header["X-INVX-REQUEST-UID"] = []string{uid}
	req.Header["X-INVX-TIMESTAMP"] = []string{ts}

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do GET %s: %w", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read GET %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d for %s: %s", res.StatusCode, path, string(raw))
	}
	return raw, nil
}
