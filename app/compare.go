package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/pkg/errors"
)

const (
	comparisonTitle         = "NDCG comparison"
	comparisonColumnQueryID = "query_id"
	comparisonSummaryLabel  = "mean NDCG"
	comparisonNDCGFormat    = "%.4f"

	// comparisonNDCGWidth is the rendered width of a formatted NDCG ("0.0000"),
	// the minimum width of an algorithm column.
	comparisonNDCGWidth = 6
	// comparisonColumnGap separates the rendered columns.
	comparisonColumnGap = 3
)

// algorithmSummary is one algorithm's aggregate score across every term it was
// evaluated with.
type algorithmSummary struct {
	Algorithm algorithm.SearchAlgorithm
	Terms     int
	MeanNDCG  float64
}

// Compare evaluates every test term with each of the given algorithms against
// the same index, logs their full-corpus relevance scores, then prints a table
// comparing each algorithm's NDCG. An empty algorithms slice evaluates every
// registered algorithm.
func (a *App) Compare(ctx context.Context, algorithms []algorithm.SearchAlgorithm) error {
	if len(algorithms) == 0 {
		algorithms = algorithm.AllSearchAlgorithms()
	}

	return a.withElasticsearch(ctx, func(ctx context.Context, esClient dpEsClient.Client) error {
		evaluations, err := a.evaluateTerms(ctx, esClient, algorithms)
		if err != nil {
			return err
		}
		return reportComparison(evaluations)
	})
}

// reportComparison prints the NDCG of every term and algorithm, followed by
// each algorithm's mean NDCG, so the strongest algorithm is readable at a
// glance.
func reportComparison(evaluations []termEvaluation) error {
	if len(evaluations) == 0 {
		ui.Warning("no terms were evaluated, so there is nothing to compare")
		return nil
	}

	ui.Section(comparisonTitle)

	if err := writeComparisonTable(os.Stdout, evaluations); err != nil {
		return err
	}

	fmt.Println()
	return nil
}

// summariseEvaluations returns each algorithm's mean NDCG across the terms it
// was evaluated with, in the order the algorithms first appear in evaluations.
func summariseEvaluations(evaluations []termEvaluation) []algorithmSummary {
	order := make([]algorithm.SearchAlgorithm, 0, len(evaluations))
	totals := make(map[algorithm.SearchAlgorithm]float64, len(evaluations))
	counts := make(map[algorithm.SearchAlgorithm]int, len(evaluations))

	for _, evaluation := range evaluations {
		if _, seen := counts[evaluation.Algorithm]; !seen {
			order = append(order, evaluation.Algorithm)
		}
		totals[evaluation.Algorithm] += evaluation.NDCG
		counts[evaluation.Algorithm]++
	}

	summaries := make([]algorithmSummary, 0, len(order))
	for _, algo := range order {
		summaries = append(summaries, algorithmSummary{
			Algorithm: algo,
			Terms:     counts[algo],
			MeanNDCG:  totals[algo] / float64(counts[algo]),
		})
	}

	return summaries
}

// writeComparisonTable renders the NDCG of every term as a row with one column
// per algorithm, followed by a mean NDCG row.
func writeComparisonTable(w io.Writer, evaluations []termEvaluation) error {
	if len(evaluations) == 0 {
		return nil
	}

	summaries := summariseEvaluations(evaluations)
	termIDs, ndcgByTerm := indexEvaluations(evaluations)

	labelWidth := widestString(append([]string{comparisonColumnQueryID, comparisonSummaryLabel}, termIDs...))
	columnWidths := make([]int, 0, len(summaries))
	for _, summary := range summaries {
		columnWidths = append(columnWidths, max(len(summary.Algorithm), comparisonNDCGWidth))
	}

	var table strings.Builder

	writeComparisonRow(&table, labelWidth, columnWidths, comparisonColumnQueryID, algorithmNames(summaries))

	for _, id := range termIDs {
		cells := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			cells = append(cells, fmt.Sprintf(comparisonNDCGFormat, ndcgByTerm[id][summary.Algorithm]))
		}
		writeComparisonRow(&table, labelWidth, columnWidths, id, cells)
	}

	table.WriteString(strings.Repeat("-", comparisonRowWidth(labelWidth, columnWidths)))
	table.WriteString("\n")

	means := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		means = append(means, fmt.Sprintf(comparisonNDCGFormat, summary.MeanNDCG))
	}
	writeComparisonRow(&table, labelWidth, columnWidths, comparisonSummaryLabel, means)

	if _, err := io.WriteString(w, table.String()); err != nil {
		return errors.Wrap(err, "failed to write comparison table")
	}

	return nil
}

// indexEvaluations returns the term ids in first-appearance order alongside
// each term's NDCG by algorithm.
func indexEvaluations(evaluations []termEvaluation) (
	termIDs []string, ndcgByTerm map[string]map[algorithm.SearchAlgorithm]float64) {
	termIDs = make([]string, 0, len(evaluations))
	ndcgByTerm = make(map[string]map[algorithm.SearchAlgorithm]float64, len(evaluations))

	for _, evaluation := range evaluations {
		byAlgorithm, seen := ndcgByTerm[evaluation.Term.ID]
		if !seen {
			byAlgorithm = make(map[algorithm.SearchAlgorithm]float64)
			ndcgByTerm[evaluation.Term.ID] = byAlgorithm
			termIDs = append(termIDs, evaluation.Term.ID)
		}
		byAlgorithm[evaluation.Algorithm] = evaluation.NDCG
	}

	return termIDs, ndcgByTerm
}

// algorithmNames returns the summarised algorithms' names, as the table's
// column headings.
func algorithmNames(summaries []algorithmSummary) []string {
	names := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		names = append(names, string(summary.Algorithm))
	}
	return names
}

// writeComparisonRow writes a left-aligned label followed by right-aligned
// cells padded to the given column widths.
func writeComparisonRow(table *strings.Builder, labelWidth int, columnWidths []int, label string, cells []string) {
	fmt.Fprintf(table, "%-*s", labelWidth, label)
	for i, cell := range cells {
		fmt.Fprintf(table, "%*s", columnWidths[i]+comparisonColumnGap, cell)
	}
	table.WriteString("\n")
}

// comparisonRowWidth returns the rendered width of a full table row.
func comparisonRowWidth(labelWidth int, columnWidths []int) int {
	width := labelWidth
	for _, columnWidth := range columnWidths {
		width += columnWidth + comparisonColumnGap
	}
	return width
}

// widestString returns the length of the longest of the given strings.
func widestString(values []string) int {
	widest := 0
	for _, value := range values {
		widest = max(widest, len(value))
	}
	return widest
}
