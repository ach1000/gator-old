-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at, updated_at, name, url, user_id;

-- name: GetFeedsByUser :many
SELECT id, created_at, updated_at, name, url, user_id
FROM feeds
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetAllFeeds :many
SELECT feeds.id, feeds.created_at, feeds.updated_at, feeds.name, feeds.url, feeds.user_id, users.name as user_name
FROM feeds
INNER JOIN users ON feeds.user_id = users.id
ORDER BY feeds.created_at DESC;