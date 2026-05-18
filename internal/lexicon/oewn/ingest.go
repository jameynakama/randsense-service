package oewn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jameynakama/randsense/internal/store"
)

// SourceName tags every row this package writes. Stored in the `source`
// column so future ingest passes (mlconjug3 for verb morphology, SUBTLEX
// for frequency, etc.) can reconcile rows by lemma+source.
const SourceName = "oewn-2025"

// Stats reports per-POS row counts from a successful Ingest run, plus a
// Skipped count covering both filtered-out lemmas (failed AllowLemma) and
// entries with a POS code we don't handle.
type Stats struct {
	Nouns      int
	Verbs      int
	Adjectives int
	Adverbs    int
	Skipped    int
}

// Ingest streams r as OEW WN-LMF XML, applies AllowLemma, dispatches each
// entry to the per-POS table by Entry.POS, and inserts. The whole run is
// one transaction -- any error rolls back. Each per-POS table is
// truncated first so re-runs produce identical state regardless of prior
// content (idempotency by clean slate).
//
// Pass the gzipped XML pre-wrapped in a gzip.Reader if you're reading
// data/oewn-2025/english-wordnet-2025.xml.gz; Ingest itself only cares
// that it gets parseable XML bytes.
func Ingest(ctx context.Context, pool *pgxpool.Pool, r io.Reader) (Stats, error) {
	var stats Stats

	tx, err := pool.Begin(ctx)
	if err != nil {
		return stats, fmt.Errorf("Ingest, pool.Begin: %v", err)
	}
	defer tx.Rollback(ctx)

	q := store.New(tx)

	// TODO: Add other word types as they are implemented
	err = q.TruncateNouns(ctx)
	if err != nil {
		return stats, fmt.Errorf("Ingest, TruncateNouns: %v", err)
	}

	err = Parse(r, func(e Entry) error {
		return ingestEntry(ctx, q, e, &stats)
	})
	if err != nil {
		return stats, fmt.Errorf("Ingest, Parse: %v", err)
	}

	return stats, tx.Commit(ctx)
}

// ingestEntry routes a single Entry to its table. POS codes we don't yet
// handle (verb, adjective, satellite, adverb) increment Skipped; 2d will
// flesh out those branches.
func ingestEntry(ctx context.Context, q *store.Queries, e Entry, stats *Stats) error {
	if !AllowLemma(e.Lemma) {
		stats.Skipped++
		return nil
	}

	switch e.POS {
	case "n":
		infl, err := nounInflectionsJSON(e.Forms)
		if err != nil {
			return err
		}
		err = q.InsertNoun(ctx, store.InsertNounParams{
			Lemma:       e.Lemma,
			Inflections: infl,
			Source:      SourceName,
		})
		if err != nil {
			return err
		}
		stats.Nouns++
	case "v", "a", "s", "r":
		// TODO: Handled in sub-slice 2d.
		stats.Skipped++
	default:
		// Anything else (proper-name codes, unknowns) gets skipped.
		stats.Skipped++
	}
	return nil
}

// nounInflectionsJSON converts the parser's Forms slice into the JSONB
// shape stored on the nouns row: {"plural": "geese"}. Empty Forms returns
// the empty object so the column never holds NULL or invalid JSON.
//
// OEW only attaches Form children to irregular plurals; regular nouns
// arrive with empty Forms and fall back to morphology rules at inflection
// time (M4).
func nounInflectionsJSON(forms []string) ([]byte, error) {
	if len(forms) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]string{"plural": forms[0]})
}
