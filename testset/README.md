# Search Relevance Test Set

Data for evaluating our search algorithms with **NDCG@K**.

This directory stores three things:

1. **Documents** — the test data (pages that could be returned by a search).
2. **Terms** — the searches we want to evaluate.
3. **Judgements** — the human-assigned relevance of a document *for a given term*
   (this will be used to score the algorithm's performance against the Idealised Discounted Cumulative Gain @ K - IDCG@K).

Relevance is graded on a fixed **0–4** scale (NDCG graded relevance: 0 = not
relevant … 4 = perfect match).

## Layout

```txt
testset/
  documents/
    cpi-latest.json                       # one file per document; filename == document id
    accountancy-services-timeseries.json
    growth-dataset.json
  terms/
    cpi-latest.json                       # one file per term; filename == term id
    data.json
    growth-figures.json
    stat.json
  judgements/
    cpi-latest.json                       # one file per term: that term's full answer key
    data.json
    growth-figures.json
    stat.json
  search-data-structures.drawio           # class/ER diagram (open in diagrams.net)
  README.md
```

## How the three structures relate

`Judgement` is the link between a `Term` and a `Document`. Relevance is a property
of the **pair**, not of either side alone — so it lives on the judgement.

```txt
  TERM   ──< judged by >──  JUDGEMENT  ──< about >──  DOCUMENT
  (what      term_id ────────┘    └──────── doc_id    (what was
   we                                                    returned)
   searched)
```

- A **term** can have many judgements (one per document scored for it).
- A **document** can appear in many terms' judgements (it's a shared pool —
  stored once, referenced by id).
- A **judgement** points at exactly one term (`term_id`) and one document
  (`doc_id`) and carries the `relevance` grade.

Join keys:

- `judgements[*].doc_id`   → `documents[*].id`
- a judgement file's `term_id` → `terms[*].id`

### What each file shows

**`documents/<id>.json`** — one searchable page, mirroring our prod search index
schema (`EsModel`). `id` is a field used as the filename and join key;
the prod index itself identifies items by `uri`.

```json
{
  "id": "cpi-latest",
  "type": "bulletin",
  "uri": "/economy/inflationandpriceindices/bulletins/consumerpriceinflation/april2026",
  "job_id": "",
  "search_index": "ons",
  "cdid": "",
  "dataset_id": "",
  "edition": "",
  "keywords": ["cpi", "cpih", "inflation", "consumer price inflation"],
  "meta_description": "The Consumer Prices Index ... inflation rates for the UK.",
  "release_date": "2026-05-21T06:00:00.000Z",
  "summary": "Price indices, percentage changes and weights ...",
  "title": "Consumer price inflation, UK: April 2026",
  "topics": ["economy", "inflationandpriceindices"],
  "cancelled": false,
  "finalised": true,
  "published": true,
  "language": "en",
  "survey": "",
  "canonical_topic": "inflationandpriceindices",
  "population_type": null,
  "dimensions": []
}
```

**`terms/<id>.json`** — one search we evaluate. Describes the user's intent.
`id` is a plain slug used as the filename and as the join key. `query` is the
raw search term(s) as a user would type them into `ons.gov.uk/search`.

```json
{
  "id": "cpi-latest",
  "query": "cpi latest",
  "description": "Most recent CPI bulletin"
}
```

**`judgements/<term>.json`** — one term's answer key: which documents are
relevant, and how much (0 = not relevant … 4 = perfect match). The file is keyed
by `term_id`; each entry references a document and its grade.

```json
{
  "term_id": "cpi-latest",
  "judgements": [
    { "doc_id": "cpi-latest", "relevance": 4 },
    { "doc_id": "accountancy-services-timeseries", "relevance": 1 },
    { "doc_id": "growth-dataset", "relevance": 2 }
  ]
}
```

## Adding to the test set

### Add a document

1. Create `documents/<id>.json` matching the prod index schema (`EsModel` —
   `type`, `uri`, `keywords`, `summary`, `title`, `topics`, `release_date`, etc.),
   plus a top-level `id` used as the join key.
2. `id` must be unique and equal the filename (except for the `.json` extension).
3. Adding a document does nothing on its own, it only matters once a judgement
   references it.

### Add a term

1. Create `terms/<id>.json` with `id`, `term`, `description`. `term` is the
   raw search term(s) (e.g. `cpi latest`); `id` is a plain slug.
2. Create the matching `judgements/<id>.json` (see below). A term with no
   judgements cannot be scored.

### Add / change a judgement

1. Open the term's file in `judgements/` (or create it: `{ "term_id": "...", "judgements": [] }`).
2. Add `{ "doc_id": "<existing document id>", "relevance": <0-4> }`.
3. The `doc_id` **must** already exist in `documents/` — no dangling references.
4. To re-score, edit the `relevance` value in place.

## Conventions & validation

The assembled set must pass these checks on load:

- **filename == id** for documents and terms.
- **No duplicate ids** within `documents/` or `terms/`.
- **No dangling references**: every `doc_id` resolves to a `documents/` file, and
  every judgement file's `term_id` resolves to a `terms/` file.
- **In-range grades**: every `relevance` is an integer from 0 to 4.

## Why split it this way

One file per entity keeps future changes small and reviewable, avoids merge conflicts on a
single large blob, and maps 1:1 to the data model: a loader iterates over the three
directories and reassembles them into `documents`, `terms`, and `judgements`
arrays. Judgements are grouped per term because that's the unit NDCG iterates over
and the unit we add most often.
