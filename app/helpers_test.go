package app

import (
	"context"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
)

const (
	docNameCPI     = "cpi-latest"
	docNameGrowth  = "growth-dataset"
	errConnRefused = "connection refused"
	errDiskError   = "disk error"

	testExportQuery    = "cpi, latest"
	testCPITitle       = "Consumer price inflation, UK"
	testCPIURI         = "/economy/cpi"
	testGrowthTitle    = "Growth"
	testGrowthURI      = "/economy/growth"
	testShortCPITitle  = "CPI"
	testShortCPIURI    = "/cpi"
	testShortGrowthURI = "/growth"
)

// fakeStore is a [stream.Stream[stream.Item]] implementation for injecting test data.
type fakeStore struct {
	items       []stream.Item
	itemsByID   map[string]stream.Item
	getErr      error
	listErr     error
	putErr      error          // when set, Put returns it
	putCalls    *[]stream.Item // when non-nil, Put appends each written item
	deleteErr   error
	deleteCalls *[]string
}

func (f fakeStore) Get(_ context.Context, id string) (stream.Item, error) {
	return f.itemsByID[id], f.getErr
}
func (f fakeStore) List(context.Context) ([]stream.Item, error) { return f.items, f.listErr }
func (f fakeStore) Put(_ context.Context, _ string, item stream.Item) error {
	if f.putErr != nil {
		return f.putErr
	}
	if f.putCalls != nil {
		*f.putCalls = append(*f.putCalls, item)
	}
	return nil
}
func (f fakeStore) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.deleteCalls != nil {
		*f.deleteCalls = append(*f.deleteCalls, id)
	}
	return nil
}

// sampleItems returns a small fixed set of items for the loader tests.
func sampleItems() []stream.Item {
	return []stream.Item{
		{Name: docNameCPI, Body: []byte(`{"title":"CPI"}`)},
		{Name: docNameGrowth, Body: []byte(`{"title":"Growth"}`)},
	}
}
