-- name: InsertNoun :exec
INSERT INTO nouns (lemma, inflections, source)
VALUES ($1, $2, $3)
ON CONFLICT (lemma, source) DO NOTHING;

-- name: TruncateNouns :exec
TRUNCATE nouns RESTART IDENTITY CASCADE;

-- name: CountNouns :one
SELECT COUNT(*) FROM nouns;

-- name: GetNounByLemma :one
SELECT * FROM nouns
WHERE lemma = $1;

-- name: GetRandomNoun :one
SELECT * FROM nouns
WHERE active
ORDER BY random()
LIMIT 1;
