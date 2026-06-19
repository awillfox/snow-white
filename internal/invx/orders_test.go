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

func stubClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := New("pub", "sec", host, srv.Client())
	c.baseURL = srv.URL
	return c
}

func TestSendOrderSerializesDecimalsAndReturnsOrderID(t *testing.T) {
	var body map[string]json.RawMessage
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":{"orderId":12345}}`))
	})
	id, err := c.SendOrder(context.Background(), SendOrderInput{
		Symbol: "BTCTHB", Side: Buy, Type: Limit,
		LimitPrice: 7000, Value: 500000, ClientOrderID: 42,
	})
	require.NoError(t, err)
	require.Equal(t, int64(12345), id)
	// decimals serialized as bare numbers, side/orderType/timeInForce as ints
	require.JSONEq(t, `70.00`, string(body["limitPrice"]))
	require.JSONEq(t, `5000.00`, string(body["value"]))
	require.JSONEq(t, `0`, string(body["side"]))
	require.JSONEq(t, `2`, string(body["orderType"]))
	require.JSONEq(t, `1`, string(body["timeInForce"]))
	require.JSONEq(t, `42`, string(body["clientOrderId"]))
}

func TestSendOrderAPIError(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1001","message":"Reject transaction"}`))
	})
	_, err := c.SendOrder(context.Background(), SendOrderInput{Symbol: "BTCTHB", Side: Buy, Type: Market, Value: 100})
	require.Error(t, err)
	require.Contains(t, err.Error(), "1001")
}

func TestOpenOrdersParsesStringEnums(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":[{
			"side":"Buy","orderId":1,"price":"1000.00000000","quantity":"0.01000000",
			"symbol":"BTCTHB","orderType":"Limit","clientOrderId":42,"orderState":"Working",
			"receiveDateTime":"2023-05-03T00:00:00.646Z","origQuantity":"0.01000000",
			"quantityExecuted":"0.00000000","avgPrice":"0.00000000"}]}`))
	})
	got, err := c.OpenOrders(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, Buy, got[0].Side)
	require.Equal(t, Limit, got[0].Type)
	require.Equal(t, "Working", got[0].State)
	require.Equal(t, int64(100000), got[0].Price)       // 1000.00 -> satang
	require.Equal(t, int64(1000000), got[0].OrigQuantity) // 0.01 -> x1e8
	require.Equal(t, int64(42), got[0].ClientOrderID)
}

func TestAccountBalanceParses(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":[{"product":"BTC","amount":"1.50000000","hold":"0.25000000"}]}`))
	})
	got, err := c.AccountBalance(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "BTC", got[0].Product)
	require.Equal(t, int64(150000000), got[0].Amount) // 1.5 -> x1e8
	require.Equal(t, int64(25000000), got[0].Hold)
}
