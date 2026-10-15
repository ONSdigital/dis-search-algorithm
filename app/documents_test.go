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
	testDocuments = []stream.Item{
		{Name: docNameCPI, Body: []byte(`{"title":"CPI"}`)},
		{Name: docNameGrowth, Body: []byte(`{"title":"Growth"}`)},
	}
)

func TestGetDocuments(t *testing.T) {
	Convey("Given a document store containing valid document JSON", t, func() {
		ctx := context.Background()
		items := []stream.Item{
			{Name: "document-1", Body: []byte(`{"id":"document-1","uri":"/one","title":"One"}`)},
			{Name: "document-2", Body: []byte(`{"id":"document-2","uri":"/two","title":"Two"}`)},
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
					{ID: "document-1", URI: "/one", Title: "One"},
					{ID: "document-2", URI: "/two", Title: "Two"},
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
