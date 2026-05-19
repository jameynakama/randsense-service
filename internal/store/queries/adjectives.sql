-- name: InsertAdjective :exec
INSERT INTO adjectives (lemma, inflections, source)
VALUES ($1, $2, $3)
ON CONFLICT (lemma, source) DO NOTHING;

-- name: TruncateAdjectives :exec
TRUNCATE adjectives RESTART IDENTITY CASCADE;

-- name: CountAdjectives :one
SELECT COUNT(*) FROM adjectives;

-- name: GetAdjectiveByLemma :one
SELECT * FROM adjectives
WHERE lemma = $1;

-- name: GetRandomAdjective :one
SELECT * FROM adjectives
WHERE active
ORDER BY random()
LIMIT 1;
