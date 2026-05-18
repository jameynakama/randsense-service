package oewn_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jameynakama/randsense/internal/lexicon/oewn"
	"github.com/jameynakama/randsense/internal/store"
)

// TestIngest is an integration test: it runs the real Ingest pipeline
// against the testdata/sample.xml fixture and checks the resulting DB
// state. The fixture has 14 LexicalEntries: 5 nouns pass AllowLemma,
// 1 noun (.22-caliber) is filtered, 8 verbs/adj/adv are Skipped until 2d.
func TestIngest(t *testing.T) {
	ctx := context.Background()

	f, err := os.Open(filepath.Join("testdata", "sample.xml"))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	stats, err := oewn.Ingest(ctx, testPool, f)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	wantNouns := 5
	wantSkipped := 14 - wantNouns

	if stats.Nouns != wantNouns {
		t.Errorf("Stats.Nouns: got %d, want %d", stats.Nouns, wantNouns)
	}
	if stats.Skipped != wantSkipped {
		t.Errorf("Stats.Skipped: got %d, want %d", stats.Skipped, wantSkipped)
	}

	// Spot-check the database state via the generated count query.
	q := store.New(testPool)
	count, err := q.CountNouns(ctx)
	if err != nil {
		t.Fatalf("CountNouns: %v", err)
	}
	if count != int64(wantNouns) {
		t.Errorf("CountNouns: got %d, want %d", count, wantNouns)
	}

	word, err := q.GetNounByLemma(ctx, "goose")
	if err != nil {
		t.Fatalf("GetNounByLemma: %v", err)
	}
	var infl map[string]string
	err = json.Unmarshal(word.Inflections, &infl)
	if err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := infl["plural"]
	if !ok {
		t.Errorf("Plural: could not find plural inflection for %s", word.Lemma)
	}
	if v != "geese" {
		t.Errorf("Plural: got %s; want %s", v, "geese")
	}

	word, err = q.GetNounByLemma(ctx, "Microsoft")
	if err != nil {
		t.Fatalf("Microsoft, error fetching: %v", err)
	}
	if word.Lemma != "Microsoft" {
		t.Errorf("CapitalTest: got %s; want %s", word.Lemma, "Microsoft")
	}
}
