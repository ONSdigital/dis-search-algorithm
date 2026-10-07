package settings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	es7 "github.com/elastic/go-elasticsearch/v7"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/testcontainers/testcontainers-go/modules/elasticsearch"

	"github.com/ONSdigital/dis-search-algorithm/settings"
)

func TestGetSearchIndexSettings(t *testing.T) {
	Convey("Given the embedded search index settings", t, func() {
		Convey("When GetSearchIndexSettings is called", func() {
			data := settings.GetSearchIndexSettings()

			Convey("Then it should return non-empty valid JSON", func() {
				So(data, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(data, &parsed), ShouldBeNil)
			})

			Convey("Then the JSON should contain top-level settings and mappings keys", func() {
				var parsed map[string]any
				So(json.Unmarshal(data, &parsed), ShouldBeNil)
				So(parsed["settings"], ShouldNotBeNil)
				So(parsed["mappings"], ShouldNotBeNil)
			})
		})
	})
}

func TestSearchIndexSettingsAcceptedByElasticsearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test: requires Docker")
	}

	ctx := t.Context()

	container, err := elasticsearch.Run(
		ctx,
		"elasticsearch:7.10.1",
	)
	if err != nil {
		t.Fatalf("failed to start elasticsearch container: %v", err)
	}
	t.Cleanup(func() {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("warning: failed to terminate ES container: %v", err)
		}
	})

	esClient, err := es7.NewClient(es7.Config{
		Addresses: []string{container.Settings.Address},
	})
	if err != nil {
		t.Fatalf("failed to create ES client: %v", err)
	}

	Convey("Given the embedded search index settings", t, func() {
		Convey("When the settings are used to create an Elasticsearch index", func() {
			res, err := esClient.Indices.Create(
				"test-settings-validation",
				esClient.Indices.Create.WithBody(bytes.NewReader(settings.GetSearchIndexSettings())),
			)
			if res != nil {
				defer res.Body.Close()
			}

			Convey("Then Elasticsearch should accept the settings without error", func() {
				So(err, ShouldBeNil)
				So(res.IsError(), ShouldBeFalse)
			})
		})
	})
}
