# Notes from a cursory look (2026-05-11)

You asked for ideas, not a full review, so this is opinionated and skips ceremony. Read what's interesting, bin the rest. References are `path:line` so you can jump.

## What I saw

Django app, NLM SPECIALIST Lexicon (the UMLS one — explains the medical-jargon dominance, that's *its actual job*) ingested via `randsense/management/commands/ingest_lexicon.py` into per-category tables in `randsense/models.py`. Grammar is a homegrown PCFG-ish DSL in `randsense/grammar.txt`, parsed by `randsense/util/parsing.py`, expanded into a flat list of POS tags, then each tag filled from the DB by `Sentence.get_random_word`, then inflected by hand in `randsense/util/inflections.py`. Curation via Django admin + voting endpoints in `api/views.py`. That part is excellent for this kind of project — don't change it.

## Latent bugs worth a sweep

1. **`parse_grammar_file` is dead code or broken** (`randsense/util/parsing.py:19-31`). It opens, reads, splits on `\n` to get a list, then passes the list to `parse_grammar`, which does `text.split("\r\n")` on it. That'll AttributeError. Real hot path goes through `ApiSettings.save()` with a string, so this works in practice — but the function is misleading and should either be fixed or deleted.

2. **Crash on empty `next_choices`** (`randsense/util/parsing.py:69-72`). The Bernoulli loop builds `next_choices`; if every roll fails, the next line `random.choice(next_choices)` raises IndexError. Rare-but-real, especially with low weights.

3. **Probabilities don't mean what they look like** (same file). `A -> B 0.7 | C 0.3` reads like "70% B, 30% C" but the code rolls each weight independently: 0.7 means "B is eligible 70% of the time," 0.3 means "C is eligible 30% of the time," then it picks uniformly from whichever survive. So both eligible = 50/50, only B eligible = B, only C eligible = C, neither = crash (see #2). I'd be very surprised if this matches your intent on `grammar.future.txt`.

   **Fix:**
   ```python
   weights, cleaned = [], []
   for choice in grammar[level]:
       try:
           weights.append(float(choice[-1]))
           cleaned.append(choice[:-1])
       except ValueError:
           weights.append(1.0)
           cleaned.append(choice)
   next_choice = random.choices(cleaned, weights=weights, k=1)[0]
   ```

4. **`get_random_word` random PK loop** (`randsense/models.py:140-150`). Gets earliest+latest PKs, picks `random.randint`, retries until hit. If `active=True` and `rank > threshold` filter out most rows, this loops a lot. `ORDER BY random() LIMIT 1` is fine for tables of this size, or precompute a "selectable PKs" list per category and cache it (invalidate on word-removal vote crossing threshold).

## The big architectural thing: flatten-then-reconstruct

This is the one I'd flag hardest. After grammar expansion, the diagram is a flat list of POS tags. The tree structure (which words belong to which constituent) is gone. `inflect_nouns` then scans forward from each `det` to find the next noun, and `inflect_verbs` scans backward from each verb to find a subject. This works for `det adj noun verb` but falls over the moment anything nests:

- Relative clauses: "the dog [that the cat saw] ran" — flat scan attaches `ran` to `cat`.
- Coordinated subjects: "the dog and the cat run" — agreement is plural but neither subject is.
- PP modifiers: "the man with the telescopes runs" — flat scan picks up `telescopes` as the subject, pluralizes wrong.

Your `grammar.future.txt` already wants lists, coordination, sentence modifiers — all of which will break the flat-list inflector.

**Suggestion:** keep the parse tree. Replace `Sentence.diagram` (currently `ArrayField` of strings) with a JSONField holding a tree node structure — each non-terminal node has a constituent label and a list of children. Inflection walks the tree: NP nodes propagate number to their head; VP agrees with its sibling NP. As a bonus, persisted trees make "why is this sentence wrong?" debuggable — look at the diagram, see which production fired weirdly.

## Grammar DSL options

You currently maintain a small hand-rolled parser for a hand-rolled PCFG syntax. Three options ranked by upheaval:

- **Keep the DSL, use Lark.** Lark is a real parser library; you define your grammar grammar (meta!) once and Lark gives you a typed parse tree. Drops the `\r\n` fragility, gets you proper error messages on malformed grammar input. The admin-curation flow keeps working.
- **NLTK CFG/PCFG.** `nltk.CFG.fromstring()` reads BNF natively, `nltk.PCFG` handles weighted productions, and there are generation helpers built in. You'd rewrite `grammar.txt` into NLTK's syntax (very close to yours), and a chunk of `parsing.py` evaporates. Cost: NLTK is a chunky dependency.
- **Just write Python data structures.** Lose the DSL, gain debuggability and `mypy` coverage. Grammar productions become a `dict[str, list[tuple[float, list[str]]]]`. The admin grammar-editing UX gets worse, but you can mitigate with a custom admin form. Probably not the right call for you given how much you use the admin.

If the project survives a long time, Lark is the boring-and-correct answer.

## Subcategorization (the real fix for "ditransitive nasty")

Your grammar treats verbs as `intran | tran | ditran | link` but in reality each verb has specific argument frames:

- *devour* — transitive (refuses a bare `the dog devoured`).
- *give* — ditransitive (refuses `she gave the book` with no recipient).
- *put* — needs a location PP (refuses `she put the book`).
- *sleep* — refuses objects entirely.

That's why your prep phrases are gnarly: you're trying to do general grammar without per-verb subcat data.

**Important caveat for your aesthetic:** randsense is meant to produce grammatically sound nonsense. That means you want *syntactic frames* but explicitly NOT *selectional restrictions*. "The dog devoured the philosophy" is the dream output. "She gave the moon to a sneeze" is the dream output. The frames tell you you need an object; they should NOT tell you the object must be animate/concrete.

**Suggestion:** import **VerbNet** (free, downloadable). Each verb maps to a Levin class with explicit syntactic frames (NP V NP, NP V NP PP[to], etc.) and thematic roles with selectional restrictions ([+animate], [+concrete]). Take the syntactic frames, ignore the selectional restrictions — or use them inverted as a "make-it-weirder" knob. Your ingest grows a `frames` JSON field on `Verb`. Generation inverts: pick a verb first, then expand using one of *its* syntactic frames. "She put the book on the table" still happens, but so does "she put the philosophy on a Tuesday" — and that's the point.

FrameNet is the heavier-duty version of the same idea if you ever want semantic frames as well, though for grammatically-sound-nonsense the lighter syntactic layer of VerbNet is probably enough.

## Inflection scaling

Hand-rolled English morphology will keep biting you on edge cases (geese, oxen, lay/laid, sing/sang/sung, the past participles of strong verbs). Two paths:

1. **Use libraries**: `inflect` for plurals/articles, `mlconjug3` for verb conjugation. Both have decades of edge cases baked in.
2. **Trust the lexicon**. SPECIALIST already encodes inflected forms in `<inflVars>` tags — that's how `cotton-roll gingivitides` ends up in there. Your ingest stores `inflections` as JSON; expand the keys you grab. For tenses SPECIALIST doesn't cover (progressive, perfect), generate at lookup time using `mlconjug3` and cache back into the JSON.

Option 2 fits your current architecture better.

## The lexicon problem itself

The medical bias isn't filterable — it's structural. SPECIALIST *is* the UMLS lexicon. You're not going to win that fight by tuning frequency thresholds.

- **Replace it with WordNet** (free, sense-tagged, includes basic frames). You lose some of SPECIALIST's morphological richness but gain a non-medical baseline.
- **Or keep SPECIALIST but positive-whitelist with SUBTLEX-US.** SUBTLEX is subtitle-derived word frequencies — it reflects what people *say* rather than what gets written into Wikipedia/PubMed. Intersect SPECIALIST entries with SUBTLEX above some threshold; everything outside that window is dead to you. This is the actual fix for "common medical words filtered out, weird ones wormed in" — your frequency source was probably written-corpus-derived, which is why the medical terms scored low.
- **Or both.** WordNet + VerbNet for general English; SPECIALIST as a side resource for a "medical mode" toggle.

## LLM angle (you mentioned, here's a realistic version)

You assumed LLM labeling would take "a billion years." It won't — Haiku via the Batch API can chew through 100K-word labeling jobs in a few hours for under a tenner. Two specific places it could help without diluting the "deterministic generator made of grammar and lexicon" vibe:

- **Offline curation**: per-word labels (`{is_common: bool, register: 'common'|'medical'|'archaic'|'technical', confidence: float}`). Store on the model, surface in admin as a filter, use to set `active`. Replaces a year of manual voting in an afternoon.
- **Grammar tuning loop**: generate N sentences with current weights, ask LLM "rate naturalness 1-5 with reason," gradient-descend or just hand-tune the weights based on aggregate scores. Quietly self-improving.

What I'd **not** do: replace the generator with an LLM. The aesthetic of the project is "deterministic grammar produces grammatical absurdity." A neural net would just give you bland fluent output, which is worse.

## Suggested order of operations

If you do come back to it, roughly:

1. Fix the three latent bugs in `parsing.py` (twenty minutes, mostly mechanical).
2. Replace flat-list diagram with a tree on `Sentence` (this is the unlock — most other improvements ride on it).
3. SUBTLEX-US intersection on the existing lexicon — biggest visible quality jump for least effort.
4. Lark for the grammar DSL.
5. VerbNet ingest + frame-based generation.
6. LLM batch labeling pass for finer curation.

## Things to deliberately not touch

- The Django admin curation workflow. It's load-bearing and it's *good*.
- The Singleton config models. Cursed-but-fine.
- The voting endpoints. Lovely human-in-the-loop primitive.
- The XML format — it's data, it's fine, you parse it once into the DB.

---

(*This file is gitignored locally via `.git/info/exclude`. Delete it whenever you've taken what you want from it.*)
