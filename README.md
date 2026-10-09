# dis-search-algorithm

A comprehensive tool for testing and comparing search algorithm relevance across different configurations and datasets.

For how the tooling is structured (the data model, the commands, and how scoring works), see [ARCHITECTURE.md](./ARCHITECTURE.md).

## Quick Start

```bash
# 1. Run Docker (we prefer colima) 
colima start

# 2. Run tool
make compare
```

## Commands

The tool has three commands. You can run them without building, using `go run .` from the repository root.

### Prerequisites

- **Go** installed (see the version in [`go.mod`](./go.mod)).
- **Docker running** for `compare` and `export`: they start a throwaway Elasticsearch via testcontainers. We prefer colima (`colima start`). `import` does **not** need Docker.
- Run from the **repository root**: `import` writes judgement files to `testset/judgements/` relative to the working directory.

### compare

Evaluate every term with one or more algorithms against the same index, logging DCG, IDCG and NDCG per term, then compare each algorithm's NDCG side by side (needs Docker).

Every algorithm is evaluated unless `--algorithms`/`-a` is given:

```sh
# all algorithms (default)
go run . compare

# one algorithm
go run . compare --algorithms baseline

# several algorithms
go run . compare --algorithms=baseline,unweighted
```

Names are matched case-insensitively, duplicates are ignored, and the columns follow the canonical algorithm order rather than the order you list them in. An unrecognised name is rejected before Elasticsearch is started, so a typo fails immediately instead of silently reporting baseline results under another name. Run `go run . compare --help` for the available algorithms.

The run ends with an NDCG table, one row per term and one column per algorithm:

```text
term_id         baseline   unweighted
cpi-latest         0.5317       0.5317
data               0.4200       0.4200
growth-figures     0.6942       0.6942
stat               0.0000       0.0000
--------------------------------------
mean NDCG          0.4115       0.4115
```

> [!NOTE]
> Scores are calculated over the whole corpus: the search requests every document, DCG covers the full ranked list, and IDCG covers every judged grade. There is no `@K` cutoff yet.

### export

Run the same evaluation with the baseline algorithm and write each ranked result with its current judgement to a CSV (needs Docker). The CSV has no algorithm column, so the export is single-algorithm. The output path is required:

```sh
go run . export judgements -o results.csv
```

### import

Read a re-scored CSV in the export format and merge the new grades back into the judgements (no Docker). The input path is required:

```sh
go run . import judgements -i results.csv
```

The usual loop is: `export judgements` to a CSV, edit the `current_relevance` column, then `import judgements` from it. Add `-v`/`--verbose` to any command for extra output.

## Development

### Running Tests

```bash
# All tests
make test

# With coverage
make test-coverage

# With race detection
make test-race
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Security audit
make audit

# All checks
make check
```

### Tooling

We use some tooling for development that you will need to install.

#### Linting

For running lint checks against the JSON files you will need to run Node > v20 (or use the version in the .nvmrc file) and have [prettier](https://github.com/prettier/prettier) installed:

```sh
npm install -g prettier
```

For running lint checks against Go template files for JSON - we are currently experimenting with our own linting tool, [dis-json-template-linter](https://github.com/ONSdigital/dis-json-template-linter).

#### Integration testing

To ensure we're producing valid ElasticSearch queries, we run them against a docker run instance of ElasticSearch using testcontainers.

To get setup, follow our guidance on [using testcontainers](https://github.com/ONSdigital/dp-component-test/blob/main/README.md#using-testcontainers)

If you're already setup, you will just need to ensure a docker daemon is running, for example via `colima start`.

To skip these tests you can pass the `-short` flag to the go test command, e.g.

```sh
go test ./... -short
```
