# Search Algorithm

This package contains the necessary code to build requests, searches and queries to ElasticSearch 7.10.

It is currently not:

- version agnostic
- provider agnostic

This may be added later following further rounds of testing.

## Definitions

Due to the overlap in nomenclature, and for absolute clarity, this library defines the following terms:

- request
- search
- query
- term

A request is a singular request sent to ElasticSearch to retrieve information. This can be made up of many searches.

A search is defined by the ElasticSearch client as `client.Search` and consists of a `Header` and a `Body`.

A query is the body of the search and is formed of JSON partials. This *also* includes a 'query' attribute as well as others.

A term is the user supplied query term with which to search by. For example "inflation" or "latest cpi statistics".

## Usage

The package is structured in three layers. Start at whichever level matches your need:

| Type              | Purpose                                                             |
|-------------------|---------------------------------------------------------------------|
| `RequestRegistry` | Pre-builds all algorithm variants; vends a `RequestBuilder` by name |
| `RequestBuilder`  | Combines multiple searches into a `[]client.Search` msearch payload |
| `SearchBuilder`   | Pairs a header with a query body; returns a single `client.Search`  |
| `QueryBuilder`    | Renders a named query template into a JSON body                     |

---

### Sending a full multi-search request

#### Using the Registry

`NewRequestRegistry` pre-builds all requested algorithm variants at startup. Call `GetRequestBuilder` at request time to retrieve the right one without re-allocating templates:

```go
import "github.com/ONSdigital/dis-search-algorithm/algorithm"

registry := algorithm.NewRequestRegistry([]algorithm.SearchAlgorithm{
    algorithm.SearchAlgorithmBaseline,
    algorithm.SearchAlgorithmUnweighted,
})

// later, per request:
builder, err := registry.GetRequestBuilder(algorithm.SearchAlgorithmBaseline)
if err != nil {
    // handle
}

params := &algorithm.SearchParameters{
    Term: "employment",
    From: 0,
    Size: 10,
}

searches, err := builder.BuildRequest(ctx, params)
if err != nil {
    // handle
}

// searches is []client.Search - pass directly to the dp-elasticsearch client
```

Currently available Search Algorithms are:

| Algorithm Name | Description                                  |
|----------------|----------------------------------------------|
| baseline       | The current baseline weighted term algorithm |
| unweighted     | Search without any weightings in at all      |

#### Direct builder construction

When you only need one algorithm and do not need the Registry:

```go
builder, err := algorithm.NewSearchRequestBuilderBaseline()
if err != nil {
    // handle
}

searches, err := builder.BuildRequest(ctx, params)
// searches is []client.Search
```

### Building a single search (header + query)

Use a `SearchBuilder` when you need one `client.Search` pair rather than a full msearch slice:

```go
builder, err := algorithm.NewSearchBuilderBaseline()
if err != nil {
    // handle
}

search, err := builder.BuildSearch(ctx, params)
// search is a client.Search with Header and Query fields populated
```

### Building a query body only

Use a `QueryBuilder` directly when you need the raw JSON query body without an msearch header, for example to call `_search` or `_validate/query`:

```go
builder, err := algorithm.NewBaselineTermQueryBuilder()
if err != nil {
    // handle
}

body, err := builder.BuildQuery(ctx, params)
// body is minified JSON: {"query":{...},"sort":[...],"size":10,...}
```

Available query builders:

| Builder                             | Query name           | Template                                                           |
|-------------------------------------|----------------------|--------------------------------------------------------------------|
| `NewBaselineTermQueryBuilder()`     | `baseline-term`      | Weighted term match with function_score, sort, suggest, source     |
| `NewUnweightedTermQueryBuilder()`   | `unweighted-term`    | Term match without scoring boosts, sort, suggest, source           |
| `NewBrowseQueryBuilder()`           | `browse`             | Match-all with filters and sort, no term weighting                 |
| `NewCountContentTypeQueryBuilder()` | `count-content-type` | Term or match-all with content-type aggregation, `size: 0`         |
| `NewCountTopicItemsQueryBuilder()`  | `count-topic-items`  | Term or match-all with topic aggregation and filters, `size: 0`    |
| `NewCountDistinctQueryBuilder()`    | `count-distinct`     | Term or match-all filtered to documents that have a `topics` field |

---

### `SearchParameters` reference

| Field            | Type       | Description                                                                               |
|------------------|------------|-------------------------------------------------------------------------------------------|
| `Term`           | `string`   | The search term; empty string triggers a match-all where supported                        |
| `From`           | `int`      | Pagination offset                                                                         |
| `Size`           | `int`      | Number of results to return                                                               |
| `Index`          | `string`   | Overrides the default index (`ons`) for the msearch header                                |
| `SortBy`         | `string`   | One of `release_date`, `release_date_asc`, `title`, `first_letter`; defaults to relevance |
| `ReleasedAfter`  | `Date`     | Lower bound for `release_date` range filter                                               |
| `ReleasedBefore` | `Date`     | Upper bound for `release_date` range filter                                               |
| `Topic`          | `[]string` | Topic URI prefixes to filter by                                                           |
| `URIPrefix`      | `string`   | Restricts results to a URI prefix                                                         |
| `URIs`           | `[]string` | Exact URI filter                                                                          |
| `Types`          | `[]string` | Content type filter                                                                       |
| `CDIDs`          | `[]string` | CDID filter                                                                               |
| `DatasetIDs`     | `[]string` | Dataset ID filter                                                                         |
| `Highlight`      | `bool`     | Enables the highlight clause in the query                                                 |

## Reference fixture files

The tests in this package use reference fixture files to detect unintended changes to query output. The fixtures live in `testdata` and are committed to source control.

### When to update

Update the fixtures whenever you **intentionally** change a query template or scoring config. A failing test means the rendered output has changed - review the diff before deciding whether to accept it.

### How to update

Run the tests with the `-update` flag from the `algorithm/` directory:

```sh
cd algorithm
go test ./... -update
```

This rewrites all `testdata/**/*.json` files with the current rendered output.

To update a single fixture:

```sh
go test -run <testFunctionName> -update
```

### Adding a fixture for a new builder

1. Add a `Convey("Then it should match the reference fixture", ...)` block to the builder's test, referencing a new file path under `testdata`.
2. Run `go test -run <testFunctionName> -update` to generate the file.
3. Commit both the test change and the new fixture together.
