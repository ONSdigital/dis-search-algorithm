package algorithm

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	es7 "github.com/elastic/go-elasticsearch/v7"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/elasticsearch"

	"github.com/ONSdigital/dis-search-algorithm/settings"
)

var update = flag.Bool("update", false, "update reference fixture files")

const testTerm = "employment"

func normaliseJSON(t *testing.T, data []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("normaliseJSON: invalid JSON: %v", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("normaliseJSON: marshal failed: %v", err)
	}
	return append(out, '\n')
}

func logDiff(t *testing.T, referencePath string, actual []byte) {
	t.Helper()

	tmp, err := os.CreateTemp("", "query-actual-*.json")
	if err != nil {
		t.Logf("logDiff: could not create temp file: %v", err)
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(actual); err != nil {
		t.Logf("logDiff: could not write temp file: %v", err)
		return
	}
	tmp.Close()

	out, _ := exec.Command("diff", "-u", referencePath, tmp.Name()).CombinedOutput()
	if len(out) > 0 {
		t.Logf("diff (reference vs actual):\n%s", out)
	}
}

func assertMatchesReference(t *testing.T, referencePath string, actual []byte) {
	t.Helper()
	normalised := normaliseJSON(t, actual)
	if *update {
		if err := os.WriteFile(referencePath, normalised, 0600); err != nil {
			t.Fatalf("failed to update reference fixture file %s: %v", referencePath, err)
		}
	}
	expected, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("reference fixture file missing: %s (run with -update to create)", referencePath)
	}
	if !bytes.Equal(normalised, expected) {
		logDiff(t, referencePath, normalised)
	}
	So(string(normalised), ShouldEqual, string(expected))
}

func runElasticSearchContainer(ctx context.Context, t *testing.T) (*elasticsearch.ElasticsearchContainer, *es7.Client) {
	t.Helper()

	container, err := elasticsearch.Run(
		ctx,
		"elasticsearch:7.10.1",
		testcontainers.WithEnv(map[string]string{
			"ES_LOG_STYLE": "file", // do not emit logs to container stdout
			"logger.level": "WARN", // or "ERROR"
		}),
	)
	if err != nil {
		t.Fatalf("failed to start elasticsearch container: %v", err)
	}

	esClient, err := es7.NewClient(es7.Config{
		Addresses: []string{container.Settings.Address},
	})
	if err != nil {
		t.Fatalf("failed to create ES client: %v", err)
	}

	t.Cleanup(func() {
		delRes, _ := esClient.Indices.Delete([]string{testIndex})
		if delRes != nil {
			// No error check is acceptable here as the body will cleaned up during the test run
			_ = delRes.Body.Close()
		}
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("warning: failed to terminate ES container: %v", err)
		}
	})

	// Create the index with the real settings/mappings so sort fields are recognised.
	createRes, err := esClient.Indices.Create(
		testIndex,
		esClient.Indices.Create.WithBody(bytes.NewReader(settings.GetSearchIndexSettings())),
	)
	if err != nil {
		t.Fatalf("failed to create test index: %v", err)
	}
	// No error check is acceptable here as the body will cleaned up during the test run
	_ = createRes.Body.Close()

	return container, esClient
}

func TestNewQueryBuilder(t *testing.T) {
	Convey("Given valid template patterns", t, func() {
		Convey("When NewQueryBuilder is called", func() {
			builder, err := NewQueryBuilder(QueryBaselineTerm, "templates/queries/termQuery.tmpl")

			Convey("Then it should return a QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryBaselineTerm))
			})
		})
	})

	Convey("Given an invalid template pattern", t, func() {
		Convey("When NewQueryBuilder is called", func() {
			builder, err := NewQueryBuilder(QueryBaselineTerm, "templates/queries/nonexistent.tmpl")

			Convey("Then it should return an error and no builder", func() {
				So(err, ShouldNotBeNil)
				So(builder, ShouldBeNil)
			})
		})
	})
}

func TestBuildQueryBaselineTerm(t *testing.T) {
	Convey("Given a QueryBuilder built from the baseline term query", t, func() {
		builder, err := NewBaselineTermQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/baseline_term_employment.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestBuildQueryUnweightedTerm(t *testing.T) {
	Convey("Given a QueryBuilder built from the unweighted term query", t, func() {
		builder, err := NewUnweightedTermQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/unweighted_term_employment.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestGetUnweightedTermQueryBuilder(t *testing.T) {
	Convey("Given a request for the unweighted term query builder", t, func() {
		Convey("When NewUnweightedTermQueryBuilder is called", func() {
			builder, err := NewUnweightedTermQueryBuilder()

			Convey("Then it should return a configured QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryUnweightedTerm))
			})
		})
	})
}

func TestBuildQueryCountContentType(t *testing.T) {
	Convey("Given a QueryBuilder built from the count content type query", t, func() {
		builder, err := NewCountContentTypeQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/count_content_type_employment.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestGetCountContentTypeQueryBuilder(t *testing.T) {
	Convey("Given a request for the count content type query builder", t, func() {
		Convey("When NewCountContentTypeQueryBuilder is called", func() {
			builder, err := NewCountContentTypeQueryBuilder()

			Convey("Then it should return a configured QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryCountContentType))
			})
		})
	})
}

func TestBuildQueryCountDistinct(t *testing.T) {
	Convey("Given a QueryBuilder built from the count distinct items query", t, func() {
		builder, err := NewCountDistinctQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/count_distinct_employment.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestGetCountDistinctQueryBuilder(t *testing.T) {
	Convey("Given a request for the count distinct query builder", t, func() {
		Convey("When NewCountDistinctQueryBuilder is called", func() {
			builder, err := NewCountDistinctQueryBuilder()

			Convey("Then it should return a configured QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryCountDistinct))
			})
		})
	})
}

func TestBuildQueryCountTopicItems(t *testing.T) {
	Convey("Given a QueryBuilder built from the count topic items query", t, func() {
		builder, err := NewCountTopicItemsQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/count_topic_items_employment.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestGetCountTopicItemsQueryBuilder(t *testing.T) {
	Convey("Given a request for the count topic items query builder", t, func() {
		Convey("When NewCountTopicItemsQueryBuilder is called", func() {
			builder, err := NewCountTopicItemsQueryBuilder()

			Convey("Then it should return a configured QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryCountTopicItems))
			})
		})
	})
}

func TestBuildQueryBrowse(t *testing.T) {
	Convey("Given a QueryBuilder built from the browse query", t, func() {
		builder, err := NewBrowseQueryBuilder()
		So(err, ShouldBeNil)

		Convey("When BuildQuery is called with a search term", func() {
			params := &SearchParameters{
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})

			Convey("Then it should match the reference fixture", func() {
				So(err, ShouldBeNil)
				assertMatchesReference(t, "testdata/queries/browse.json", result)
			})
		})

		Convey("When BuildQuery is called with an empty term", func() {
			params := &SearchParameters{
				Term: "",
				From: 0,
				Size: 10,
			}
			result, err := builder.BuildQuery(context.Background(), params)

			Convey("Then it should return valid JSON with no error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeEmpty)

				var parsed map[string]any
				So(json.Unmarshal(result, &parsed), ShouldBeNil)
			})
		})
	})
}

func TestGetBrowseQueryBuilder(t *testing.T) {
	Convey("Given a request for the browse query builder", t, func() {
		Convey("When NewBrowseQueryBuilder is called", func() {
			builder, err := NewBrowseQueryBuilder()

			Convey("Then it should return a configured QueryBuilder with no error", func() {
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)
				So(string(builder.name), ShouldEqual, string(QueryBrowse))
			})
		})
	})
}

// This test will test all the query builders against a real Elasticsearch instance to ensure that
// the generated queries are valid and accepted by Elasticsearch. It requires Docker to be running
// and will start an Elasticsearch container for testing.
func TestQueryValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test: requires Docker")
	}

	ctx := context.Background()
	_, esClient := runElasticSearchContainer(ctx, t)

	cases := []struct {
		name    string
		builder func() (*QueryBuilder, error)
	}{
		{name: "baseline term", builder: NewBaselineTermQueryBuilder},
		{name: "unweighted term", builder: NewUnweightedTermQueryBuilder},
		{name: "count content type", builder: NewCountContentTypeQueryBuilder},
		{name: "count distinct", builder: NewCountDistinctQueryBuilder},
		{name: "count topic items", builder: NewCountTopicItemsQueryBuilder},
		{name: "browse", builder: NewBrowseQueryBuilder},
	}

	for _, tc := range cases {
		tc := tc
		Convey(fmt.Sprintf("Given a running Elasticsearch instance - %s query", tc.name), t, func() {
			queryBuilder, err := tc.builder()
			So(err, ShouldBeNil)

			fullSearch, err := queryBuilder.BuildQuery(ctx, &SearchParameters{
				Term: testTerm,
				From: 0,
				Size: 10,
			})
			So(err, ShouldBeNil)

			// Run the full search body against the real index - ES will return 400
			// on any parse/validation error in query, sort, _source, suggest, etc.
			res, _ := esClient.Search(
				esClient.Search.WithIndex(testIndex),
				esClient.Search.WithBody(bytes.NewReader(fullSearch)),
			)
			if res != nil {
				defer res.Body.Close()
			}
			var result map[string]any
			err = json.NewDecoder(res.Body).Decode(&result)
			So(err, ShouldBeNil)

			t.Logf("Elasticsearch %s response: %v", tc.name, result)

			Convey(fmt.Sprintf("Then Elasticsearch should accept the %s body as valid", tc.name), func() {
				So(err, ShouldBeNil)
				So(res.StatusCode, ShouldNotEqual, 400)
				// ES returns {"error": ...} with status 400 for malformed bodies;
				// a 200 with hits (even 0) means the entire request was valid.
				So(result["error"], ShouldBeNil)
			})
		})
	}
}
