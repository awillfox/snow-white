-- name: InsertOrder :one
INSERT INTO orders (client_uid, symbol, side, type, limit_price, quantity, mode, strategy, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
RETURNING *;

-- name: SettleOrder :exec
UPDATE orders
SET status = $2, exchange_ref = $3, reason = $4
WHERE id = $1;

-- name: ListPendingOrders :many
SELECT * FROM orders WHERE status = 'pending' AND mode = 'live' ORDER BY id ASC;

-- name: GetPosition :one
SELECT * FROM positions WHERE symbol = $1;

-- name: UpsertPosition :exec
INSERT INTO positions (symbol, qty, avg_cost, realized_pnl, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (symbol) DO UPDATE SET
    qty = EXCLUDED.qty,
    avg_cost = EXCLUDED.avg_cost,
    realized_pnl = EXCLUDED.realized_pnl,
    updated_at = now();

-- name: GetRiskState :one
SELECT * FROM risk_state WHERE day = $1;

-- name: InsertRiskState :one
INSERT INTO risk_state (day) VALUES ($1)
ON CONFLICT (day) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: UpdateRiskSpentLoss :exec
UPDATE risk_state
SET spent_today = spent_today + $2, loss_today = loss_today + $3, updated_at = now()
WHERE day = $1;

-- name: SetRiskHalted :exec
UPDATE risk_state SET halted = $2, halt_reason = $3, updated_at = now() WHERE day = $1;
