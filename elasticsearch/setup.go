package elasticsearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/ONSdigital/dis-search-algorithm/settings"
	dpEs "github.com/ONSdigital/dp-elasticsearch/v4/client"
)

// PrepareIndex creates the specified index in Elasticsearch
// with the provided settings.
func PrepareIndex(ctx context.Context, esClient dpEs.Client, indexName string) error {
	if esClient == nil {
		return errors.New("elasticsearch client is nil")
	}

	esSettings := settings.GetSearchIndexSettings()

	// TODO: add check to see if the index already exists and delete it if necessary

	if err := esClient.CreateIndex(ctx, indexName, esSettings); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}
