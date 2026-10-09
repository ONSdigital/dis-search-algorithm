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
	termIDCPI       = "cpi-latest"
	termIDGrowth    = "growth-dataset"
	testExportQuery = "cpi, latest"
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
