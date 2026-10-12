package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
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
