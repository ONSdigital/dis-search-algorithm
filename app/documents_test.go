package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	dpEsClientMock "github.com/ONSdigital/dp-elasticsearch/v4/client/mocks"
	searchModel "github.com/ONSdigital/dp-search-api/models"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	docNameCPI         = "cpi-latest"
	docNameGrowth      = "growth-dataset"
	testGrowthTitle    = "Growth"
	testShortCPITitle  = "CPI"
	testShortCPIURI    = "/cpi"
	testShortGrowthURI = "/growth"
)

var (
	testDocuments = []Document{
		{
			ID:  docNameCPI,
			URI: testShortCPIURI,
			Item: searchModel.Item{
				Title: testShortCPITitle,
			},
		},
		{
			ID:  docNameGrowth,
			URI: testShortGrowthURI,
			Item: searchModel.Item{
				Title: testGrowthTitle,
			},
		},
	}
)

func getTestDocumentsAsStream() []stream.Item {
	items := make([]stream.Item, len(testDocuments))
	for i, doc := range testDocuments {
		items[i] = stream.Item{Name: doc.ID, Body: []byte(fmt.Sprintf(`{"id":%q, "uri":%q, "item":{"title":%q}}`, doc.ID, doc.URI, doc.Item.Title))}
	}
	return items
}

func TestGetDocuments(t *testing.T) {
	Convey("Given a document store containing valid document JSON", t, func() {
		ctx := context.Background()
		items := []stream.Item{
			{Name: "document-1", Body: []byte(`{"id":"document-1","uri":"/one","item":{"title":"One"}}`)},
			{Name: "document-2", Body: []byte(`{"id":"document-2","uri":"/two","item":{"title":"Two"}}`)},
		}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(gotCtx context.Context) ([]stream.Item, error) {
				So(gotCtx, ShouldEqual, ctx)
				return items, nil
			},
		}
		app := &App{Documents: store}

		Convey("When GetDocuments is called", func() {
			got, err := app.GetDocuments(ctx)

			Convey("Then it should return every decoded document", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, []Document{
					{ID: "document-1", URI: "/one", Item: searchModel.Item{Title: "One"}},
					{ID: "document-2", URI: "/two", Item: searchModel.Item{Title: "Two"}},
				})
				So(store.ListCalls(), ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given the document store fails to list", t, func() {
		listErr := errors.New("document store unavailable")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return nil, listErr
			},
		}
		app := &App{Documents: store}

		Convey("When GetDocuments is called", func() {
			got, err := app.GetDocuments(context.Background())

			Convey("Then it should return a wrapped list error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list documents")
				So(err.Error(), ShouldContainSubstring, listErr.Error())
				So(got, ShouldBeNil)
			})
		})
	})

	Convey("Given the document store contains malformed JSON", t, func() {
		store := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(context.Context) ([]stream.Item, error) {
				return []stream.Item{{Name: "broken-document", Body: []byte(`{"id":`)}}, nil
			},
		}
		app := &App{Documents: store}

		Convey("When GetDocuments is called", func() {
			got, err := app.GetDocuments(context.Background())

			Convey("Then it should identify the document that could not be parsed", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to parse document "broken-document"`)
				So(got, ShouldBeNil)
			})
		})
	})
}

func TestGetMatchingDocumentsForTermPagination(t *testing.T) {
	for _, count := range []int{0, 1, 50, 51, 100} {
		Convey(fmt.Sprintf("Given a search with %d matching documents", count), t, func() {
			ctx := context.Background()
			ids := make([]string, count)
			for i := range ids {
				ids[i] = fmt.Sprintf("document-%d", i)
			}

			store := &fileStoreMocks.StreamMock[stream.Item]{
				GetFunc: func(_ context.Context, id string) (stream.Item, error) {
					return stream.Item{Body: []byte(fmt.Sprintf(`{"id":%q}`, id))}, nil
				},
			}
			client := &dpEsClientMock.ClientMock{
				MultiSearchFunc: func(_ context.Context, searches []dpEsClient.Search, _ *dpEsClient.QueryParams) ([]byte, error) {
					if len(searches) == 0 {
						return nil, fmt.Errorf("expected a term search")
					}
					var query struct {
						From int `json:"from"`
						Size int `json:"size"`
					}
					if err := json.Unmarshal(searches[0].Query, &query); err != nil {
						return nil, err
					}
					end := query.From + query.Size
					if end > count {
						end = count
					}
					hits := make([]map[string]string, 0)
					if query.From < count {
						for _, id := range ids[query.From:end] {
							hits = append(hits, map[string]string{"_id": id})
						}
					}
					return json.Marshal(map[string]any{"responses": []any{map[string]any{"hits": map[string]any{"hits": hits}}}})
				},
			}

			Convey("When matching documents are retrieved", func() {
				got, err := (&App{Documents: store}).getMatchingDocumentsForTerm(ctx, client, Term{ID: "term", Value: "test"})

				Convey("Then all documents are returned in ranked order", func() {
					So(err, ShouldBeNil)
					gotIDs := make([]string, len(got))
					for i, document := range got {
						gotIDs[i] = document.ID
					}
					So(gotIDs, ShouldResemble, ids)
				})

				Convey("Then pages are requested at successive offsets until a short response", func() {
					So(err, ShouldBeNil)
					calls := client.MultiSearchCalls()
					So(calls, ShouldHaveLength, count/50+1)
					for page, call := range calls {
						var query struct {
							From int `json:"from"`
							Size int `json:"size"`
						}
						So(json.Unmarshal(call.Searches[0].Query, &query), ShouldBeNil)
						So(query.From, ShouldEqual, page*50)
						So(query.Size, ShouldEqual, 50)
					}
				})
			})
		})
	}
}

func TestPutDocument(t *testing.T) {
	Convey("Given a document missing from the store", t, func() {
		ctx := context.Background()
		document := Document{ID: "document-1", URI: "/one", Item: searchModel.Item{Title: "One", Topics: []string{"economy"}}}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
			PutFunc: func(context.Context, string, stream.Item) error { return nil },
		}
		app := &App{Documents: store}

		Convey("When PutDocument is called", func() {
			updated, err := app.PutDocument(ctx, document)

			Convey("Then it stores the document as indented JSON with a trailing newline", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeTrue)
				calls := store.PutCalls()
				So(calls, ShouldHaveLength, 1)
				So(calls[0].Ctx, ShouldEqual, ctx)
				So(calls[0].ID, ShouldEqual, document.ID)
				So(calls[0].Item.Name, ShouldEqual, document.ID)
				want, marshalErr := json.MarshalIndent(document, "", "  ")
				So(marshalErr, ShouldBeNil)
				So(string(calls[0].Item.Body), ShouldEqual, string(want)+"\n")
			})
		})
	})

	Convey("Given an unchanged document already in the store", t, func() {
		document := Document{ID: "document-1", URI: "/one", Item: searchModel.Item{Topics: []string{"economy"}}}
		body, err := json.Marshal(document)
		So(err, ShouldBeNil)
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Body: body}, nil
			},
		}

		Convey("When PutDocument is called", func() {
			updated, err := (&App{Documents: store}).PutDocument(context.Background(), document)

			Convey("Then it skips the write", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeFalse)
				So(store.PutCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given a document whose metadata has changed", t, func() {
		document := Document{ID: "document-1", URI: "/one", Item: searchModel.Item{Topics: []string{"economy", "prices"}}}
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{Body: []byte(`{"id":"document-1","uri":"/one","item":{"topics":["economy"]}}`)}, nil
			},
			PutFunc: func(context.Context, string, stream.Item) error { return nil },
		}

		Convey("When PutDocument is called", func() {
			updated, err := (&App{Documents: store}).PutDocument(context.Background(), document)

			Convey("Then it replaces the stored metadata", func() {
				So(err, ShouldBeNil)
				So(updated, ShouldBeTrue)
				So(store.PutCalls(), ShouldHaveLength, 1)
				var saved Document
				So(json.Unmarshal(store.PutCalls()[0].Item.Body, &saved), ShouldBeNil)
				So(saved, ShouldResemble, document)
			})
		})
	})

	Convey("Given the store fails to read the existing document", t, func() {
		readErr := errors.New("read failed")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, readErr
			},
		}

		Convey("When PutDocument is called", func() {
			updated, err := (&App{Documents: store}).PutDocument(context.Background(), Document{ID: "document-1"})

			Convey("Then it returns the read error without writing", func() {
				So(updated, ShouldBeFalse)
				So(errors.Is(err, readErr), ShouldBeTrue)
				So(store.PutCalls(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given the store fails to write a new document", t, func() {
		writeErr := errors.New("write failed")
		store := &fileStoreMocks.StreamMock[stream.Item]{
			GetFunc: func(context.Context, string) (stream.Item, error) {
				return stream.Item{}, stream.ErrItemNotFound
			},
			PutFunc: func(context.Context, string, stream.Item) error { return writeErr },
		}

		Convey("When PutDocument is called", func() {
			updated, err := (&App{Documents: store}).PutDocument(context.Background(), Document{ID: "document-1"})

			Convey("Then it returns the write error", func() {
				So(updated, ShouldBeFalse)
				So(errors.Is(err, writeErr), ShouldBeTrue)
				So(store.PutCalls(), ShouldHaveLength, 1)
			})
		})
	})
}
