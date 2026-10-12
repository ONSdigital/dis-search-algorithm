package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	termIDCPI       = "cpi-latest"
	termIDGrowth    = "growth-dataset"
	testExportQuery = "cpi, latest"
)

var (
	testTerms = []stream.Item{
		{Name: termIDCPI, Body: []byte(`{"id":"cpi-latest"}`)},
		{Name: termIDGrowth, Body: []byte(`{"id":"growth-dataset"}`)},
	}
)

func TestGetTerm(t *testing.T) {
	Convey("Given a term stored as JSON", t, func() {
		ctx := context.Background()
		const termID = "term-123"
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(gotCtx context.Context, gotID string) (stream.Item, error) {
				So(gotCtx, ShouldEqual, ctx)
				So(gotID, ShouldEqual, termID)
				return stream.Item{Body: []byte(`{"id":"term-123","term":"employment"}`)}, nil
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerm retrieves the term", func() {
			got, err := app.GetTerm(ctx, termID)

			Convey("Then the JSON term should be returned", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, Term{ID: termID, Value: "employment"})
				So(store.GetCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the term store returns an error", t, func() {
		storeErr := errors.New("term store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, storeErr
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerm retrieves the term", func() {
			got, err := app.GetTerm(context.Background(), "term-123")

			Convey("Then the store error should be returned", func() {
				So(err, ShouldEqual, storeErr)
				So(got, ShouldResemble, Term{})
			})
		})
	})

	Convey("Given the term store returns malformed JSON", t, func() {
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Body: []byte(`{"ID":`)}, nil
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerm decodes the term", func() {
			got, err := app.GetTerm(context.Background(), "term-123")

			Convey("Then the JSON decoding error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(got, ShouldResemble, Term{})
			})
		})
	})
}

func TestGetTerms(t *testing.T) {
	Convey("Given a term store containing valid term JSON", t, func() {
		ctx := context.Background()
		items := []stream.Item{
			{Name: "employment", Body: []byte(`{"id":"employment","term":"jobs","description":"Employment data"}`)},
			{Name: "prices", Body: []byte(`{"id":"prices","term":"inflation","description":"Price data"}`)},
		}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(_ context.Context) ([]stream.Item, error) {
				return items, nil
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerms is called", func() {
			got, err := app.GetTerms(ctx)

			Convey("Then it should return every decoded term", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, []Term{
					{ID: "employment", Value: "jobs", Description: "Employment data"},
					{ID: "prices", Value: "inflation", Description: "Price data"},
				})
				So(store.ListCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the term store fails to list", t, func() {
		storeErr := errors.New("term store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return nil, storeErr
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerms is called", func() {
			got, err := app.GetTerms(context.Background())

			Convey("Then it should return a wrapped list error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list terms")
				So(err.Error(), ShouldContainSubstring, storeErr.Error())
				So(got, ShouldBeNil)
			})
		})
	})

	Convey("Given the term store contains malformed JSON", t, func() {
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{{Name: "broken-term", Body: []byte(`{"id":`)}}, nil
			},
		}
		app := &App{Terms: store}

		Convey("When GetTerms is called", func() {
			got, err := app.GetTerms(context.Background())

			Convey("Then it should report which term could not be parsed", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to parse term "broken-term"`)
				So(got, ShouldBeNil)
			})
		})
	})
}

func TestPutTerm(t *testing.T) {
	Convey("Given a term that is not yet in the store", t, func() {
		ctx := context.Background()
		term := Term{ID: "employment", Value: "jobs", Description: "Employment data"}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
			PutFunc: func(gotCtx context.Context, gotID string, item stream.Item) error {
				So(gotCtx, ShouldEqual, ctx)
				So(gotID, ShouldEqual, term.ID)
				So(item.Name, ShouldEqual, term.ID)
				var gotTerm Term
				So(json.Unmarshal(item.Body, &gotTerm), ShouldBeNil)
				So(gotTerm, ShouldResemble, term)
				return nil
			},
		}
		app := &App{Terms: store}

		Convey("When PutTerm is called", func() {
			updated, err := app.PutTerm(ctx, term)

			Convey("Then it should create the term", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeTrue)
				So(store.PutCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the same term is already in the store", t, func() {
		term := Term{ID: "employment", Value: "jobs", Description: "Employment data"}
		body, err := marshalTerm(term)
		So(err, ShouldBeNil)
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Name: term.ID, Body: body}, nil
			},
		}
		app := &App{Terms: store}

		Convey("When PutTerm is called", func() {
			updated, err := app.PutTerm(context.Background(), term)

			Convey("Then it should not write the unchanged term", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeFalse)
				So(store.PutCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given a different term with the same ID is in the store", t, func() {
		term := Term{ID: "employment", Value: "new jobs", Description: "Updated description"}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{
					Body: []byte(`{"id":"employment","term":"old jobs","description":"Old description"}`),
				}, nil
			},
			PutFunc: func(context.Context, string, stream.Item) error { return nil },
		}
		app := &App{Terms: store}

		Convey("When PutTerm is called", func() {
			updated, err := app.PutTerm(context.Background(), term)

			Convey("Then it should replace the stored term", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeTrue)
				So(store.PutCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the term store returns an unexpected read error", t, func() {
		storeErr := errors.New("term store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, storeErr
			},
		}
		app := &App{Terms: store}

		Convey("When PutTerm is called", func() {
			updated, err := app.PutTerm(context.Background(), Term{ID: "employment"})

			Convey("Then it should return the read error without writing", func() {
				So(updated, ShouldBeFalse)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to check existing term "employment"`)
				So(err.Error(), ShouldContainSubstring, storeErr.Error())
				So(store.PutCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given the term store fails to write", t, func() {
		writeErr := errors.New("disk full")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
			PutFunc: func(context.Context, string, stream.Item) error { return writeErr },
		}
		app := &App{Terms: store}

		Convey("When PutTerm is called", func() {
			updated, err := app.PutTerm(context.Background(), Term{ID: "employment", Value: "jobs"})

			Convey("Then it should return the write error", func() {
				So(updated, ShouldBeFalse)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to write term "employment"`)
				So(err.Error(), ShouldContainSubstring, writeErr.Error())
			})
		})
	})
}

func TestDeleteTerm(t *testing.T) {
	Convey("Given a term store", t, func() {
		ctx := context.Background()
		const termID = "employment"
		store := &fileStoreMocks.StreamMock[stream.Item]{
			DeleteFunc: func(gotCtx context.Context, gotID string) error {
				So(gotCtx, ShouldEqual, ctx)
				So(gotID, ShouldEqual, termID)
				return nil
			},
		}
		app := &App{Terms: store}

		Convey("When DeleteTerm is called", func() {
			err := app.DeleteTerm(ctx, termID)

			Convey("Then it should delete the term", func() {
				So(err, ShouldBeNil)
				So(store.DeleteCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the term store fails to delete", t, func() {
		storeErr := errors.New("term store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			DeleteFunc: func(context.Context, string) error {
				return storeErr
			},
		}
		app := &App{Terms: store}

		Convey("When DeleteTerm is called", func() {
			err := app.DeleteTerm(context.Background(), "employment")

			Convey("Then it should return the store error", func() {
				So(err, ShouldEqual, storeErr)
			})
		})
	})
}

func TestImportTermsToStore(t *testing.T) {
	Convey("Given the term store fails to list existing terms", t, func() {
		listErr := errors.New("term store unavailable")
		termStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return nil, listErr
			},
		}
		app := &App{Terms: termStore}

		Convey("When terms are imported", func() {
			err := app.importTermsToStore(context.Background(), []Term{{ID: "employment"}})

			Convey("Then the list error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list terms")
				So(err.Error(), ShouldContainSubstring, listErr.Error())
				So(termStore.GetCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given the imported term and its judgement already exist", t, func() {
		term := Term{ID: "employment", Value: "jobs", Description: "Employment data"}
		termBody, err := marshalTerm(term)
		So(err, ShouldBeNil)
		termStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{{Name: term.ID, Body: termBody}}, nil
			},
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Name: term.ID, Body: termBody}, nil
			},
		}
		judgementStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return judgementItem(term.ID, Judgement{DocID: "document-123", Relevance: 3}), nil
			},
		}
		app := &App{Terms: termStore, Judgements: judgementStore}

		Convey("When that term is imported", func() {
			err := app.importTermsToStore(context.Background(), []Term{term})

			Convey("Then it should preserve the existing data without writing or deleting", func() {
				So(err, ShouldBeNil)
				So(termStore.PutCalls(), ShouldBeEmpty)
				So(termStore.DeleteCalls(), ShouldBeEmpty)
				So(judgementStore.PutCalls(), ShouldBeEmpty)
				So(judgementStore.DeleteCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given the judgement store returns an unexpected read error", t, func() {
		term := Term{ID: "employment", Value: "jobs"}
		termBody, err := marshalTerm(term)
		So(err, ShouldBeNil)
		termStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{{Name: term.ID, Body: termBody}}, nil
			},
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Name: term.ID, Body: termBody}, nil
			},
		}
		judgementErr := errors.New("judgement store unavailable")
		judgementStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, judgementErr
			},
		}
		app := &App{Terms: termStore, Judgements: judgementStore}

		Convey("When that term is imported", func() {
			err := app.importTermsToStore(context.Background(), []Term{term})

			Convey("Then the judgement read error should stop the import", func() {
				So(err, ShouldEqual, judgementErr)
				So(termStore.PutCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given a stored term is absent from the import", t, func() {
		importedTerm := Term{ID: "employment", Value: "jobs"}
		staleTerm := Term{ID: "obsolete", Value: "old query"}
		importedBody, err := marshalTerm(importedTerm)
		So(err, ShouldBeNil)
		staleBody, err := marshalTerm(staleTerm)
		So(err, ShouldBeNil)
		termStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{{Name: staleTerm.ID, Body: staleBody}}, nil
			},
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Name: importedTerm.ID, Body: importedBody}, nil
			},
			DeleteFunc: func(context.Context, string) error { return nil },
		}
		judgementStore := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return judgementItem(importedTerm.ID), nil
			},
			DeleteFunc: func(context.Context, string) error { return nil },
		}
		app := &App{Terms: termStore, Judgements: judgementStore}

		Convey("When the new terms are imported", func() {
			err := app.importTermsToStore(context.Background(), []Term{importedTerm})

			Convey("Then the stale term and its judgement should be deleted", func() {
				So(err, ShouldBeNil)
				So(termStore.DeleteCalls(), ShouldHaveLength, 1)
				So(termStore.DeleteCalls()[0].ID, ShouldEqual, staleTerm.ID)
				So(judgementStore.DeleteCalls(), ShouldHaveLength, 1)
				So(judgementStore.DeleteCalls()[0].ID, ShouldEqual, staleTerm.ID)
			})
		})
	})
}

func TestMarshalTerm(t *testing.T) {
	Convey("Given a term with all fields populated", t, func() {
		term := Term{
			ID:          "employment",
			Value:       "jobs",
			Description: "Employment data",
		}

		Convey("When marshalTerm encodes the term", func() {
			body, err := marshalTerm(term)

			Convey("Then it should render fixture-style JSON with a trailing newline", func() {
				So(err, ShouldBeNil)
				So(string(body), ShouldEqual, "{\n"+
					"  \"id\": \"employment\",\n"+
					"  \"term\": \"jobs\",\n"+
					"  \"description\": \"Employment data\"\n"+
					"}\n")
			})

			Convey("Then the output should be valid JSON that decodes to the original term", func() {
				var decoded Term
				unmarshalErr := json.Unmarshal(body, &decoded)
				So(err, ShouldBeNil)
				So(unmarshalErr, ShouldBeNil)
				So(decoded, ShouldResemble, term)
			})
		})
	})
}
