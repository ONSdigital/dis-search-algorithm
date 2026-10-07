package app

import (
	"context"

	testElasticsearch "github.com/ONSdigital/dis-search-algorithm/elasticsearch"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	"github.com/pkg/errors"

	dpEs "github.com/ONSdigital/dp-elasticsearch/v4"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
)

const (
	indexNameDocuments = "search-documents"

	// label used in each store's log and error messages.
	labelDocument = "document"
)

// withElasticsearch starts an Elasticsearch instance, loads the test documents,
// and invokes run against the prepared client. The instance is always stopped.
func (a *App) withElasticsearch(ctx context.Context, run func(context.Context, dpEsClient.Client) error) (err error) {
	spinner := ui.NewSpinner("starting elasticsearch container for testing...")
	spinner.Start()

	testElasticSearchInstance, err := testElasticsearch.NewElasticSearchContainer()
	if err != nil {
		spinner.Stop()
		return err
	}
	spinner.Stop()
	ui.Celebrate("elasticsearch container started")

	defer func() {
		shutdownSpinner := ui.NewSpinner("shutting down elasticsearch container...")
		shutdownSpinner.Start()
		terminateErr := testElasticSearchInstance.Terminate(context.Background())
		shutdownSpinner.Stop()
		if err == nil && terminateErr != nil {
			err = terminateErr
		}
	}()

	testEsURL, err := testElasticSearchInstance.GetURL(ctx)
	if err != nil {
		return err
	}

	esClient, err := dpEs.NewClient(dpEsClient.Config{
		Address: testEsURL,
	})
	if err != nil {
		return err
	}
	ui.Info("elasticsearch client initialised")

	if err := a.loadIndexes(ctx, esClient); err != nil {
		return err
	}
	ui.Info("index %s created", indexNameDocuments)

	if err := a.loadStores(ctx, esClient); err != nil {
		return err
	}

	return run(ctx, esClient)
}

// storeTarget pairs a fixture store with the index it loads into and the
// label used in its log and error messages.
type storeTarget struct {
	store     stream.Stream[stream.Item]
	indexName string
	label     string
}

// storeTargets is the single source of truth for which store loads into
// which index.
func (a *App) storeTargets() []storeTarget {
	return []storeTarget{
		{a.Documents, indexNameDocuments, labelDocument},
	}
}

// loadIndexes creates the Elasticsearch index for every store target.
func (a *App) loadIndexes(ctx context.Context, esClient dpEsClient.Client) error {
	for _, t := range a.storeTargets() {
		if err := testElasticsearch.PrepareIndex(ctx, esClient, t.indexName); err != nil {
			return err
		}
	}
	return nil
}

// loadStores loads every store target into its Elasticsearch index.
func (a *App) loadStores(ctx context.Context, esClient dpEsClient.Client) error {
	for _, t := range a.storeTargets() {
		if err := a.loadStore(ctx, esClient, t.store, t.indexName, t.label); err != nil {
			return err
		}
	}
	return nil
}

// loadStore reads every item from store and indexes each one into indexName.
// label is used in log and error messages, e.g. "document".
func (a *App) loadStore(ctx context.Context, esClient dpEsClient.Client, store stream.Stream[stream.Item], indexName, label string) error {
	items, err := store.List(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to list %ss", label)
	}

	for _, item := range items {
		if err := esClient.AddDocument(ctx, indexName, item.Name, item.Body, nil); err != nil {
			return errors.Wrapf(err, "failed to index %s %q", label, item.Name)
		}
	}

	ui.Info("loaded %d %ss into index %s", len(items), label, indexName)
	return nil
}
