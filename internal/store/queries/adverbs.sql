-- name: InsertAdverb :exec
INSERT INTO adverbs (lemma, inflections, source)
VALUES ($1, $2, $3)
ON CONFLICT (lemma, source) DO NOTHING;

-- name: TruncateAdverbs :exec
TRUNCATE adverbs RESTART IDENTITY CASCADE;

-- name: CountAdverbs :one
SELECT COUNT(*) FROM adverbs;

-- name: GetAdverbByLemma :one
SELECT * FROM adverbs
WHERE lemma = $1;

-- name: GetRandomAdverb :one
SELECT * FROM adverbs
WHERE active
ORDER BY random()
LIMIT 1;
