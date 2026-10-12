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
	termIDCPI    = "cpi-latest"
	termIDGrowth = "growth-dataset"
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
