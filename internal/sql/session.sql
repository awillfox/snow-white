-- name: InsertSessionTrack :one
INSERT INTO session_tracks (session_event, balance, event_at)
VALUES ($1, $2, now())
RETURNING *;
