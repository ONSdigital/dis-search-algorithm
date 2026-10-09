package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	errConnRefused = "connection refused"
	errDiskError   = "disk error"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

const (
	docNameAccountancy = "accountancy-services-timeseries"
	orphanDocID        = "unknown-doc"
	csvHeader          = "term_id,query,doc_id,current_relevance,title,uri"
)

// documentItems builds a corpus of documents identified only by id (import only
// reads the ids).
func documentItems(ids ...string) []stream.Item {
	items := make([]stream.Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, stream.Item{Name: id, Body: []byte(`{}`)})
	}
	return items
}

// judgementItem builds a stored judgement item for the Judgements fake store,
// serialised independently of the code under test.
func judgementItem(termID string, entries ...Judgement) stream.Item {
	body, err := json.Marshal(TermJudgements{TermID: termID, Judgements: entries})
	if err != nil {
		panic(err)
	}
	return stream.Item{Name: termID, Body: body}
}

func TestBuildTermJudgements(t *testing.T) {
	Convey("Given items that mix documents", t, func() {
		termJudgements := []TermJudgement{
			{Term: Term{ID: termIDCPI}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 3}},
			{Term: Term{ID: termIDCPI}, Document: Document{ID: orphanDocID}, Judgement: Judgement{DocID: orphanDocID, Relevance: 2}},
		}

		Convey("When the judgements are built", func() {
			compositeTermJudgements := buildTermJudgements(termJudgements)

			Convey("Then orphan rows are skipped and counted", func() {
				So(compositeTermJudgements, ShouldNotBeEmpty)
				So(compositeTermJudgements, ShouldHaveLength, 1)
				So(compositeTermJudgements, ShouldContain, TermJudgements{TermID: termIDCPI, Judgements: []Judgement{
					{DocID: docNameCPI, Relevance: 3},
					{DocID: orphanDocID, Relevance: 2},
				}})
			})
		})
	})

	Convey("Given items that mix terms", t, func() {
		termJudgements := []TermJudgement{
			{Term: Term{ID: termIDCPI}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 3}},
			{Term: Term{ID: termIDGrowth}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 2}},
		}

		Convey("When the judgements are built", func() {
			compositeTermJudgements := buildTermJudgements(termJudgements)

			Convey("Then orphan rows are skipped and counted", func() {
				So(compositeTermJudgements, ShouldNotBeEmpty)
				So(compositeTermJudgements, ShouldHaveLength, 2)
				So(compositeTermJudgements, ShouldContain, TermJudgements{TermID: termIDCPI, Judgements: []Judgement{
					{DocID: docNameCPI, Relevance: 3},
				}})
				So(compositeTermJudgements, ShouldContain, TermJudgements{TermID: termIDGrowth, Judgements: []Judgement{
					{DocID: docNameCPI, Relevance: 2},
				}})
			})
		})
	})
}

func TestValidateJudgements(t *testing.T) {
	Convey("Given a valid judgement", t, func() {
		existingDocuments := documentItems(docNameCPI)

		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return existingDocuments, nil
			},
		}

		app := &App{
			Documents: fakeDocStore,
		}

		validJudgement := TermJudgement{
			Term:      Term{ID: termIDCPI},
			Document:  Document{ID: docNameCPI},
			Judgement: Judgement{DocID: docNameCPI, Relevance: 3},
		}

		Convey("When the judgement is validated", func() {
			isValid, err := app.validateTermJudgement(context.Background(), validJudgement)

			Convey("Then the judgement is valid", func() {
				So(err, ShouldBeNil)
				So(isValid, ShouldBeTrue)
			})
		})
	})

	Convey("Given a judgement with an invalid relevance", t, func() {
		existingDocuments := documentItems(docNameCPI)

		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return existingDocuments, nil
			},
		}

		app := &App{
			Documents: fakeDocStore,
		}

		invalidJudgement := TermJudgement{
			Term:      Term{ID: termIDCPI},
			Document:  Document{ID: docNameCPI},
			Judgement: Judgement{DocID: docNameCPI, Relevance: 5},
		}

		Convey("When the judgement is validated", func() {
			isValid, err := app.validateTermJudgement(context.Background(), invalidJudgement)

			Convey("Then the judgement is invalid", func() {
				So(err, ShouldBeNil)
				So(isValid, ShouldBeFalse)
			})
		})
	})

	Convey("Given a judgement with an invalid document", t, func() {
		existingDocuments := documentItems(docNameCPI)

		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return existingDocuments, nil
			},
		}

		app := &App{
			Documents: fakeDocStore,
		}

		invalidJudgement := TermJudgement{
			Term:      Term{ID: termIDCPI},
			Document:  Document{ID: "invalid-doc"},
			Judgement: Judgement{DocID: "invalid-doc", Relevance: 3},
		}

		Convey("When the judgement is validated", func() {
			isValid, err := app.validateTermJudgement(context.Background(), invalidJudgement)

			Convey("Then the judgement is invalid", func() {
				So(err, ShouldBeNil)
				So(isValid, ShouldBeFalse)
			})
		})
	})
}

func TestImportJudgementsToStore(t *testing.T) {
	Convey("Given there are existing documents and a judgement", t, func() {
		existingDocuments := documentItems(docNameCPI, docNameGrowth, docNameAccountancy)

		existingJudgement := judgementItem(termIDCPI,
			Judgement{DocID: docNameCPI, Relevance: 4},
			Judgement{DocID: docNameAccountancy, Relevance: 1},
			Judgement{DocID: docNameGrowth, Relevance: 2},
		)

		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return existingDocuments, nil
			},
		}

		fakeJudgementStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{existingJudgement}, nil
			},
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return existingJudgement, nil
			},
			PutFunc: func(ctx context.Context, id string, item stream.Item) error {
				return nil
			},
		}

		app := &App{
			Documents:  fakeDocStore,
			Judgements: fakeJudgementStore,
		}

		Convey("When a matching judgements are imported", func() {
			importJudgements := []TermJudgement{
				TermJudgement{
					Judgement: Judgement{
						DocID:     docNameCPI,
						Relevance: 3,
					},
					Document: Document{
						ID: docNameCPI,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			}

			err := app.importJudgementsToStore(context.Background(), importJudgements)

			Convey("Then the judgement overwritten and written pretty-printed", func() {
				So(err, ShouldBeNil)
				So(fakeJudgementStore.PutCalls(), ShouldHaveLength, 1)
				So(fakeJudgementStore.PutCalls()[0].Item.Name, ShouldEqual, termIDCPI)
				So(strings.HasSuffix(string(fakeJudgementStore.PutCalls()[0].Item.Body), "\n"), ShouldBeTrue)
				So(string(fakeJudgementStore.PutCalls()[0].Item.Body), ShouldContainSubstring, "\n  \"term_id\":")

				var got TermJudgements
				So(json.Unmarshal(fakeJudgementStore.PutCalls()[0].Item.Body, &got), ShouldBeNil)
				So(got.TermID, ShouldEqual, termIDCPI)
				So(got.Judgements, ShouldResemble, []Judgement{
					{DocID: docNameCPI, Relevance: 3},
				})
			})
		})

		Convey("When a a judgement hasn't changed", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     docNameCPI,
						Relevance: 4,
					},
					Document: Document{
						ID: docNameCPI,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
				{
					Judgement: Judgement{
						DocID:     docNameAccountancy,
						Relevance: 1,
					},
					Document: Document{
						ID: docNameAccountancy,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
				{
					Judgement: Judgement{
						DocID:     docNameGrowth,
						Relevance: 2,
					},
					Document: Document{
						ID: docNameGrowth,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			})

			Convey("Then nothing is written", func() {
				So(err, ShouldBeNil)
				So(fakeJudgementStore.PutCalls(), ShouldBeEmpty)
			})
		})

		Convey("When a judgement references a document not in the corpus", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     docNameCPI,
						Relevance: 3,
					},
					Document: Document{
						ID: docNameCPI,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
				{
					Judgement: Judgement{
						DocID:     "unknown-doc",
						Relevance: 4,
					},
					Document: Document{
						ID: "unknown-doc",
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			})

			Convey("Then the orphan row is skipped and not written", func() {
				So(err, ShouldBeNil)
				So(fakeJudgementStore.PutCalls(), ShouldHaveLength, 1)
				So(string(fakeJudgementStore.PutCalls()[0].Item.Body), ShouldNotContainSubstring, orphanDocID)
			})
		})

		Convey("When a query with no existing judgement file is imported", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     docNameCPI,
						Relevance: 3,
					},
					Document: Document{
						ID: docNameCPI,
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			})

			Convey("Then a new judgement file is created", func() {
				So(err, ShouldBeNil)
				So(fakeJudgementStore.PutCalls(), ShouldHaveLength, 1)
				var got TermJudgements
				So(json.Unmarshal(fakeJudgementStore.PutCalls()[0].Item.Body, &got), ShouldBeNil)
				So(got.Judgements, ShouldResemble, []Judgement{{DocID: docNameCPI, Relevance: 3}})
			})
		})
	})

	Convey("Given the document store fails to list", t, func() {
		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return nil, errors.New(errDiskError)
			},
		}

		app := &App{
			Documents:  fakeDocumentStore,
			Judgements: &fileStoreMocks.StreamMock[stream.Item]{},
		}

		Convey("When judgements are imported", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     "unknown-doc",
						Relevance: 4,
					},
					Document: Document{
						ID: "unknown-doc",
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			})

			Convey("Then the error is wrapped and surfaced", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list documents")
			})
		})
	})

	Convey("Given the judgement store fails to write", t, func() {
		existingDocuments := documentItems(docNameCPI, docNameGrowth, docNameAccountancy)

		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return existingDocuments, nil
			},
		}

		fakeJudgementStore := &fileStoreMocks.StreamMock[stream.Item]{
			PutFunc: func(ctx context.Context, id string, item stream.Item) error {
				return errors.New(errConnRefused)
			},
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return nil, nil
			},
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				return stream.Item{}, nil
			},
		}

		app := &App{
			Documents:  fakeDocumentStore,
			Judgements: fakeJudgementStore,
		}

		Convey("When an import would change a judgement", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     "cpi-latest",
						Relevance: 3,
					},
					Document: Document{
						ID: "cpi-latest",
					},
					Term: Term{
						ID:    "cpi-latest",
						Value: "cpi latest",
					},
				},
			})

			Convey("Then the write error is wrapped and surfaced", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to write judgement")
			})
		})
	})
}
