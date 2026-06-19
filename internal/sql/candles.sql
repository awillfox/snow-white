-- name: UpsertCandle :exec
INSERT INTO candles (
    symbol, open_time, open, high, low, close, volume, inside_bid, inside_ask, source
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (symbol, open_time) DO UPDATE SET
    open       = EXCLUDED.open,
    high       = EXCLUDED.high,
    low        = EXCLUDED.low,
    close      = EXCLUDED.close,
    volume     = EXCLUDED.volume,
    inside_bid = EXCLUDED.inside_bid,
    inside_ask = EXCLUDED.inside_ask,
    source     = EXCLUDED.source;

-- name: ListCandles :many
SELECT *
FROM candles
WHERE symbol = $1
  AND open_time >= sqlc.arg(from_time)
  AND open_time <= sqlc.arg(to_time)
ORDER BY open_time ASC
LIMIT sqlc.arg('row_limit');

-- name: ListSymbols :many
SELECT DISTINCT symbol FROM candles ORDER BY symbol ASC;
