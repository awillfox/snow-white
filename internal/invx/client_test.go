package invx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTickerParsesCandles(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeaders = r.Header.Clone()
		_, _ = w.Write([]byte(`{
			"code":"0000","message":"SUCCESS",
			"data":[{"dateTime":"2023-06-14T08:47:00.000Z","high":"896789.99000000",
			"low":"800000.00000000","open":"850000.00000000","close":"896789.99000000",
			"volume":"1.50000000","insideBidPrice":"892910.64000000",
			"insideAskPrice":"896789.99000000","symbol":"BTCTHB"}]}`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	c := New("pub", "sec", host, srv.Client())
	c.baseURL = srv.URL // test override; real client builds https://host

	candles, err := c.Ticker(context.Background(), "BTCTHB")
	require.NoError(t, err)
	require.Len(t, candles, 1)
	require.Equal(t, int64(89678999), candles[0].Close) // 896789.99 -> satang
	require.Equal(t, int64(150000000), candles[0].Volume) // 1.5 -> *1e8

	require.Equal(t, "pub", gotHeaders.Get("X-INVX-APIKEY"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-SIGNATURE"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-REQUEST-UID"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-TIMESTAMP"))

	// Body sent must be the exact bytes signed (compact JSON, no trailing space).
	var probe map[string]string
	require.NoError(t, json.Unmarshal([]byte(gotBody), &probe))
	require.Equal(t, "BTCTHB", probe["symbol"])
}
