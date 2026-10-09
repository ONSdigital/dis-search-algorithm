package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	dpEsClientMock "github.com/ONSdigital/dp-elasticsearch/v4/client/mocks"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadStore(t *testing.T) {
	cases := []struct {
		label     string
		indexName string
	}{
		{labelDocument, indexNameDocuments},
	}

	for _, tc := range cases {
		testName := fmt.Sprintf("Given a %s store with two items", tc.label)
		Convey(testName, t, func() {
			app := &App{}

			Convey("When loadStore is called with a client that indexes successfully", func() {
				mockClient := &dpEsClientMock.ClientMock{
					AddDocumentFunc: func(ctx context.Context, indexName, documentID string, document []byte, opts *dpEsClient.AddDocumentOptions) error {
						return nil
					},
				}

				fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
					ListFunc: func(ctx context.Context) ([]stream.Item, error) {
						return testDocuments, nil
					},
				}

				err := app.loadStore(context.Background(), mockClient, fakeDocumentStore, tc.indexName, tc.label)

				Convey("Then it should index every item once with no error", func() {
					So(err, ShouldBeNil)
					So(len(mockClient.AddDocumentCalls()), ShouldEqual, 2)
				})

				Convey("Then it should index each item into the correct index by name", func() {
					calls := mockClient.AddDocumentCalls()
					So(calls[0].IndexName, ShouldEqual, tc.indexName)
					So(calls[0].DocumentID, ShouldEqual, docNameCPI)
					So(calls[1].IndexName, ShouldEqual, tc.indexName)
					So(calls[1].DocumentID, ShouldEqual, docNameGrowth)
				})
			})

			Convey("When the store fails to list", func() {
				mockClient := &dpEsClientMock.ClientMock{
					AddDocumentFunc: func(ctx context.Context, indexName, documentID string, document []byte, opts *dpEsClient.AddDocumentOptions) error {
						return nil
					},
				}

				fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
					ListFunc: func(ctx context.Context) ([]stream.Item, error) {
						return nil, errors.New(errDiskError)
					},
				}
				err := app.loadStore(context.Background(), mockClient, fakeDocumentStore, tc.indexName, tc.label)

				Convey("Then it should return a wrapped error and index nothing", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "failed to list "+tc.label+"s")
					So(err.Error(), ShouldContainSubstring, errDiskError)
					So(len(mockClient.AddDocumentCalls()), ShouldEqual, 0)
				})
			})

			Convey("When the client fails to index an item", func() {
				mockClient := &dpEsClientMock.ClientMock{
					AddDocumentFunc: func(ctx context.Context, indexName, documentID string, document []byte, opts *dpEsClient.AddDocumentOptions) error {
						return errors.New(errConnRefused)
					},
				}

				fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
					ListFunc: func(ctx context.Context) ([]stream.Item, error) {
						return testDocuments, nil
					},
				}
				err := app.loadStore(context.Background(), mockClient, fakeDocumentStore, tc.indexName, tc.label)

				Convey("Then it should return a wrapped error and stop at the first failure", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "failed to index "+tc.label)
					So(err.Error(), ShouldContainSubstring, docNameCPI)
					So(err.Error(), ShouldContainSubstring, errConnRefused)
					So(len(mockClient.AddDocumentCalls()), ShouldEqual, 1)
				})
			})
		})
	}
}

func TestLoadStores(t *testing.T) {
	Convey("Given an App with its document store populated", t, func() {
		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return testDocuments, nil
			},
		}
		app := &App{
			Documents: fakeDocumentStore,
		}
		mockClient := &dpEsClientMock.ClientMock{
			AddDocumentFunc: func(ctx context.Context, indexName, documentID string, document []byte, opts *dpEsClient.AddDocumentOptions) error {
				return nil
			},
		}

		Convey("When loadStores is called", func() {
			err := app.loadStores(context.Background(), mockClient)

			Convey("Then it should load every store into its index", func() {
				So(err, ShouldBeNil)
				calls := mockClient.AddDocumentCalls()
				So(len(calls), ShouldEqual, 2)
				So(calls[0].IndexName, ShouldEqual, indexNameDocuments)
			})
		})
	})

	Convey("Given an App where a store fails to list", t, func() {
		cases := []struct {
			label      string
			newApp     func() *App
			wantLoaded int
		}{
			{
				label: "documents",
				newApp: func() *App {
					return &App{
						Documents: &fileStoreMocks.StreamMock[stream.Item]{ListFunc: func(ctx context.Context) ([]stream.Item, error) {
							return nil, errors.New(errDiskError)
						}},
					}
				},
				wantLoaded: 0,
			},
		}

		for _, tc := range cases {
			Convey("When loadStores is called and the "+tc.label+" store fails", func() {
				mockClient := &dpEsClientMock.ClientMock{
					AddDocumentFunc: func(ctx context.Context, indexName, documentID string, document []byte, opts *dpEsClient.AddDocumentOptions) error {
						return nil
					},
				}

				err := tc.newApp().loadStores(context.Background(), mockClient)

				Convey("Then it should return a wrapped error naming "+tc.label+" and stop", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "failed to list "+tc.label)
					So(err.Error(), ShouldContainSubstring, errDiskError)
					So(len(mockClient.AddDocumentCalls()), ShouldEqual, tc.wantLoaded)
				})
			})
		}
	})
}

func TestLoadIndexes(t *testing.T) {
	Convey("Given a client that creates indexes successfully", t, func() {
		mockClient := &dpEsClientMock.ClientMock{
			CreateIndexFunc: func(ctx context.Context, indexName string, indexSettings []byte) error {
				return nil
			},
		}
		app := &App{}

		Convey("When loadIndexes is called", func() {
			err := app.loadIndexes(context.Background(), mockClient)

			Convey("Then it should create the documents index by name", func() {
				So(err, ShouldBeNil)
				calls := mockClient.CreateIndexCalls()
				So(len(calls), ShouldEqual, 1)
				So(calls[0].IndexName, ShouldEqual, indexNameDocuments)
			})
		})
	})

	Convey("Given a client that fails to create the first index", t, func() {
		mockClient := &dpEsClientMock.ClientMock{
			CreateIndexFunc: func(ctx context.Context, indexName string, indexSettings []byte) error {
				return errors.New(errConnRefused)
			},
		}
		app := &App{}

		Convey("When loadIndexes is called", func() {
			err := app.loadIndexes(context.Background(), mockClient)

			Convey("Then it should return a wrapped error and stop after the first index", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to create index")
				So(err.Error(), ShouldContainSubstring, errConnRefused)
				So(len(mockClient.CreateIndexCalls()), ShouldEqual, 1)
			})
		})
	})
}
