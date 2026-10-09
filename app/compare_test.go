package app

import (
	"bytes"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	. "github.com/smartystreets/goconvey/convey"
)

// comparisonFixture returns two terms evaluated with two algorithms, in the
// algorithm-major order evaluateTerms produces.
func comparisonFixture() []termEvaluation {
	return []termEvaluation{
		{Algorithm: algorithm.SearchAlgorithmBaseline, Term: Term{ID: termIDCPI}, NDCG: 0.8},
		{Algorithm: algorithm.SearchAlgorithmBaseline, Term: Term{ID: termIDGrowth}, NDCG: 0.6},
		{Algorithm: algorithm.SearchAlgorithmUnweighted, Term: Term{ID: termIDCPI}, NDCG: 0.5},
		{Algorithm: algorithm.SearchAlgorithmUnweighted, Term: Term{ID: termIDGrowth}, NDCG: 0.1},
	}
}

func TestSummariseEvaluations(t *testing.T) {
	Convey("Given two terms evaluated with two algorithms", t, func() {
		Convey("When the evaluations are summarised", func() {
			summaries := summariseEvaluations(comparisonFixture())

			Convey("Then each algorithm's mean NDCG should be returned in first-appearance order", func() {
				So(summaries, ShouldResemble, []algorithmSummary{
					{Algorithm: algorithm.SearchAlgorithmBaseline, Terms: 2, MeanNDCG: 0.7},
					{Algorithm: algorithm.SearchAlgorithmUnweighted, Terms: 2, MeanNDCG: 0.3},
				})
			})
		})
	})

	Convey("Given evaluations for a single algorithm", t, func() {
		evaluations := []termEvaluation{
			{Algorithm: algorithm.SearchAlgorithmUnweighted, Term: Term{ID: termIDCPI}, NDCG: 0.25},
		}

		Convey("When the evaluations are summarised", func() {
			summaries := summariseEvaluations(evaluations)

			Convey("Then only that algorithm should be summarised", func() {
				So(summaries, ShouldResemble, []algorithmSummary{
					{Algorithm: algorithm.SearchAlgorithmUnweighted, Terms: 1, MeanNDCG: 0.25},
				})
			})
		})
	})

	Convey("Given no evaluations", t, func() {
		Convey("When the evaluations are summarised", func() {
			summaries := summariseEvaluations(nil)

			Convey("Then no summaries should be returned", func() {
				So(summaries, ShouldBeEmpty)
			})
		})
	})
}

func TestWriteComparisonTable(t *testing.T) {
	Convey("Given two terms evaluated with two algorithms", t, func() {
		var output bytes.Buffer

		Convey("When the comparison table is written", func() {
			err := writeComparisonTable(&output, comparisonFixture())

			Convey("Then it should render an NDCG row per term and a mean NDCG row", func() {
				So(err, ShouldBeNil)
				So(output.String(), ShouldEqual, ""+
					"term_id          baseline   unweighted\n"+
					"cpi-latest         0.8000       0.5000\n"+
					"growth-dataset     0.6000       0.1000\n"+
					"--------------------------------------\n"+
					"mean NDCG          0.7000       0.3000\n")
			})
		})
	})

	Convey("Given evaluations for a single algorithm", t, func() {
		var output bytes.Buffer
		evaluations := []termEvaluation{
			{Algorithm: algorithm.SearchAlgorithmBaseline, Term: Term{ID: termIDCPI}, NDCG: 0.5},
		}

		Convey("When the comparison table is written", func() {
			err := writeComparisonTable(&output, evaluations)

			Convey("Then it should render a single algorithm column", func() {
				So(err, ShouldBeNil)
				So(output.String(), ShouldEqual, ""+
					"term_id      baseline\n"+
					"cpi-latest     0.5000\n"+
					"---------------------\n"+
					"mean NDCG      0.5000\n")
			})
		})
	})

	Convey("Given no evaluations", t, func() {
		var output bytes.Buffer

		Convey("When the comparison table is written", func() {
			err := writeComparisonTable(&output, nil)

			Convey("Then nothing should be written", func() {
				So(err, ShouldBeNil)
				So(output.String(), ShouldBeEmpty)
			})
		})
	})

	Convey("Given a writer that fails", t, func() {
		Convey("When the comparison table is written", func() {
			err := writeComparisonTable(failingWriter{}, comparisonFixture())

			Convey("Then the write error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to write comparison table")
			})
		})
	})
}
