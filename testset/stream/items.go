package stream

import (
	"encoding/json"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/testset"
	"github.com/pkg/errors"
)

const (
	// documentWriteDir is where [NewDocumentStore]'s writer persists document
	// bodies. Reads come from the algorithm package's compiled-in snapshot;
	// writes go to the on-disk copy of the same directory, resolved relative to
	// the working directory (so callers should run from the module root). A
	// written file is only visible to Get/List once re-embedded on the next
	// build.
	documentWriteDir = "testset/" + testset.DocumentFixturesDir
	// judgementWriteDir is where [NewJudgementStore]'s writer persists
	// judgement bodies.
	judgementWriteDir = "testset/" + testset.JudgementFixturesDir
	// termWriteDir is where [NewTermStore]'s writer persists term bodies.
	termWriteDir = "testset/" + testset.TermFixturesDir
)

// Item is one rendered Elasticsearch document/judgement/term body from the
// test-data store. It is the "document/judgement/term composite" the search
// algorithms emit and that the fixtures in testset/documents/judgements/terms
// capture.
//
// A document/judgement/term file is its body, so the item is modelled as the
// raw JSON keyed by name rather than a typed struct, matching how the codebase
// treats items for indexing elsewhere ([]byte).
type Item struct {
	// Name is the document/judgement/term's identifier: the file name without
	// the .json extension (e.g. "baseline_term_employment", "browse").
	Name string `json:"name"`
	// Body is the full Elasticsearch _search body as raw JSON.
	Body json.RawMessage `json:"body"`
}

// itemCodec maps a document/judgement/term file's bytes directly to/from
// [Item.Body], taking the id (filename) as [Item.Name]. Unlike [JSONCodec] it
// does not wrap the body in an enclosing object: the file content is the body
// verbatim.
var itemCodec = Codec[Item]{
	Decode: func(id string, data []byte) (Item, error) {
		if !json.Valid(data) {
			return Item{}, errors.Errorf("invalid JSON in document/judgement/term file %q", id)
		}
		minifiedData, err := algorithm.MinifyJSON(data)
		if err != nil {
			return Item{}, errors.Wrapf(err, "failed to minify document/judgement/term file %q", id)
		}
		body := make(json.RawMessage, len(minifiedData))
		copy(body, minifiedData)
		return Item{Name: id, Body: body}, nil
	},
	Encode: func(q Item) ([]byte, error) {
		if !json.Valid(q.Body) {
			return nil, errors.Errorf("invalid JSON in document/judgement/term body %q", q.Name)
		}
		return q.Body, nil
	},
}

// NewDocumentStore returns a filesystem-backed store of the rendered [Item]
// bodies committed under testset/documents.
//
// Reads are served from the testset package's embedded, compiled-in snapshot
// ([testset.DocumentFixturesFS]). Put writes to the on-disk copy at
// [documentWriteDir]; such writes are only visible to Get and List once
// re-embedded on the next build. For a database-backed store later, provide
// another [Stream] implementation.
func NewDocumentStore() Stream[Item] {
	return NewFileStore(
		testset.DocumentFixturesFS(),
		testset.DocumentFixturesDir,
		documentWriteDir,
		itemCodec,
	)
}

// NewJudgementStore returns a filesystem-backed store of the rendered [Item]
// bodies committed under testset/judgements.
//
// Reads are served from the testset package's embedded, compiled-in snapshot
// ([testset.JudgementFixturesFS]). Put writes to the on-disk copy at
// [judgementWriteDir]; such writes are only visible to Get and List once
// re-embedded on the next build. For a database-backed store later, provide
// another [Stream] implementation.
func NewJudgementStore() Stream[Item] {
	return NewFileStore(
		testset.JudgementFixturesFS(),
		testset.JudgementFixturesDir,
		judgementWriteDir,
		itemCodec,
	)
}

// NewTermStore returns a filesystem-backed store of the rendered [Item]
// bodies committed under testset/terms.
//
// Reads are served from the testset package's embedded, compiled-in snapshot
// ([testset.TermFixturesFS]). Put writes to the on-disk copy at
// [termWriteDir]; such writes are only visible to Get and List once
// re-embedded on the next build. For a database-backed store later, provide
// another [Stream] implementation.
func NewTermStore() Stream[Item] {
	return NewFileStore(
		testset.TermFixturesFS(),
		testset.TermFixturesDir,
		termWriteDir,
		itemCodec,
	)
}
