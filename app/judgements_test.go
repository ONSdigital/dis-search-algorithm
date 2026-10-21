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

// judgementItem builds a stored judgement item for the Judgements fake store,
// serialised independently of the code under test.
func judgementItem(termID string, entries ...Judgement) stream.Item {
	body, err := json.Marshal(TermJudgements{TermID: termID, Judgements: entries})
	if err != nil {
		panic(err)
	}
	return stream.Item{Name: termID, Body: body}
}

func TestGetTermJudgements(t *testing.T) {
	Convey("Given a term's judgements stored as JSON", t, func() {
		ctx := context.Background()
		const termID = "term-employment"
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(gotCtx context.Context, gotID string) (stream.Item, error) {
				return stream.Item{
					Name: termID,
					Body: []byte(`{"term_id":"term-employment","judgements":[{"doc_id":"document-123","relevance":4},{"doc_id":"document-456","relevance":2}]}`),
				}, nil
			},
		}
		app := &App{Judgements: store}

		Convey("When GetTermJudgements is called", func() {
			got, err := app.GetTermJudgements(ctx, termID)

			Convey("Then it should return the decoded term and all its judgements", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, TermJudgements{
					TermID: termID,
					Judgements: []Judgement{
						{DocID: "document-123", Relevance: 4},
						{DocID: "document-456", Relevance: 2},
					},
				})
				So(store.GetCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the judgement store returns an error", t, func() {
		storeErr := errors.New("judgement store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, storeErr
			},
		}
		app := &App{Judgements: store}

		Convey("When GetTermJudgements is called", func() {
			got, err := app.GetTermJudgements(context.Background(), "term-employment")

			Convey("Then it should return the store error", func() {
				So(err, ShouldEqual, storeErr)
				So(got, ShouldResemble, TermJudgements{})
			})
		})
	})

	Convey("Given the judgement store returns malformed JSON", t, func() {
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Body: []byte(`{"doc_id":`)}, nil
			},
		}
		app := &App{Judgements: store}

		Convey("When GetTermJudgements is called", func() {
			got, err := app.GetTermJudgements(context.Background(), "term-employment")

			Convey("Then it should return a JSON decoding error", func() {
				So(err, ShouldNotBeNil)
				So(got, ShouldResemble, TermJudgements{})
			})
		})
	})
}

func TestBuildTermJudgements(t *testing.T) {
	Convey("Given items that mix documents", t, func() {
		termJudgements := []TermJudgement{
			{Term: Term{ID: termIDCPI}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 3}},
			{Term: Term{ID: termIDCPI}, Document: Document{ID: orphanDocID}, Judgement: Judgement{DocID: orphanDocID, Relevance: 2}},
		}

		Convey("When the judgements are built", func() {
			compositeTermJudgements, err := buildTermJudgements(termJudgements)

			Convey("Then orphan rows are skipped and counted and no error occurred", func() {
				So(err, ShouldBeNil)
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
			compositeTermJudgements, err := buildTermJudgements(termJudgements)

			Convey("Then orphan rows are skipped and counted and no error occurred", func() {
				So(err, ShouldBeNil)
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

	Convey("Given items that duplicate a document for a term", t, func() {
		termJudgements := []TermJudgement{
			{Term: Term{ID: termIDCPI}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 3}},
			{Term: Term{ID: termIDCPI}, Document: Document{ID: docNameCPI}, Judgement: Judgement{DocID: docNameCPI, Relevance: 2}},
		}

		Convey("When the judgements are built", func() {
			compositeTermJudgements, err := buildTermJudgements(termJudgements)

			Convey("Then an error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(compositeTermJudgements, ShouldBeNil)
			})
		})
	})
}

func TestValidateJudgements(t *testing.T) {
	Convey("Given a valid judgement", t, func() {
		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return getTestDocumentsAsStream()[0], nil
			},
		}

		fakeTermStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return testTerms[0], nil
			},
		}

		app := &App{
			Documents: fakeDocStore,
			Terms:     fakeTermStore,
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
		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return getTestDocumentsAsStream()[0], nil
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
		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
		}

		fakeTermStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return testTerms[0], nil
			},
		}

		app := &App{
			Documents: fakeDocStore,
			Terms:     fakeTermStore,
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

	Convey("Given a judgement with an invalid term ID", t, func() {
		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return getTestDocumentsAsStream()[0], nil
			},
		}

		fakeTermStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
		}

		app := &App{
			Documents: fakeDocStore,
			Terms:     fakeTermStore,
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
		existingJudgement := judgementItem(termIDCPI,
			Judgement{DocID: docNameCPI, Relevance: 4},
			Judgement{DocID: docNameAccountancy, Relevance: 1},
			Judgement{DocID: docNameGrowth, Relevance: 2},
		)

		fakeDocStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				if id == orphanDocID {
					return stream.Item{}, stream.ErrItemNotFound
				}
				return getTestDocumentsAsStream()[0], nil
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

		fakeTermStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				if id == orphanDocID {
					return stream.Item{}, stream.ErrItemNotFound
				}
				return testTerms[0], nil
			},
		}

		app := &App{
			Documents:  fakeDocStore,
			Judgements: fakeJudgementStore,
			Terms:      fakeTermStore,
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

		Convey("When a judgement references a document not in the store", func() {
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
						ID:    termIDCPI,
						Value: "cpi latest",
					},
				},
				{
					Judgement: Judgement{
						DocID:     orphanDocID,
						Relevance: 4,
					},
					Document: Document{
						ID: orphanDocID,
					},
					Term: Term{
						ID:    termIDCPI,
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
						ID:    termIDCPI,
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

	Convey("Given the document store fails to get a document", t, func() {
		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				return stream.Item{}, errors.New(errDiskError)
			},
		}

		existingJudgement := judgementItem(termIDCPI,
			Judgement{DocID: docNameCPI, Relevance: 4},
			Judgement{DocID: docNameAccountancy, Relevance: 1},
			Judgement{DocID: docNameGrowth, Relevance: 2},
		)

		app := &App{
			Documents: fakeDocumentStore,
			Judgements: &fileStoreMocks.StreamMock[stream.Item]{
				ListFunc: func(ctx context.Context) ([]stream.Item, error) {
					return []stream.Item{existingJudgement}, nil
				},
			},
		}

		Convey("When judgements are imported", func() {
			err := app.importJudgementsToStore(context.Background(), []TermJudgement{
				{
					Judgement: Judgement{
						DocID:     orphanDocID,
						Relevance: 4,
					},
					Document: Document{
						ID: orphanDocID,
					},
					Term: Term{
						ID:    termIDCPI,
						Value: "cpi latest",
					},
				},
			})

			Convey("Then the error is wrapped and surfaced", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, errDiskError)
			})
		})
	})

	Convey("Given the judgement store fails to write", t, func() {
		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				return getTestDocumentsAsStream()[0], nil
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

		fakeTermStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
				return testTerms[0], nil
			},
		}

		app := &App{
			Documents:  fakeDocumentStore,
			Judgements: fakeJudgementStore,
			Terms:      fakeTermStore,
		}

		Convey("When an import would change a judgement", func() {
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
						ID:    termIDCPI,
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
