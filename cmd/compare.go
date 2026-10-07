package cmd

import (
	"fmt"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/app"
	"github.com/spf13/cobra"
)

const (
	compareCommandName   = "compare"
	compareAlgorithmFlag = "algorithms"
)

func compareCommand() *cobra.Command {
	var algorithmNames []string

	compareCmd := &cobra.Command{
		Use:   compareCommandName,
		Short: "Compare algorithms across terms",
		Long: fmt.Sprintf(`Compare algorithms across different terms requests with the same index.

Evaluates every test term with each selected algorithm and reports their NDCG
side by side, with each algorithm's mean NDCG.

Available algorithms: %[1]s.
Every algorithm is evaluated unless --%[2]s is given:

  search-testbed compare
  search-testbed compare --%[2]s %[3]s
  search-testbed compare --%[2]s=%[4]s`,
			strings.Join(algorithm.SearchAlgorithmNames(), ", "),
			compareAlgorithmFlag,
			algorithm.SearchAlgorithmBaseline,
			strings.Join(algorithm.SearchAlgorithmNames(), ",")),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve the names before any Elasticsearch work: Compare starts a
			// throwaway container, so a mistyped name must fail immediately.
			algorithms, err := algorithm.ParseSearchAlgorithms(algorithmNames)
			if err != nil {
				return err
			}

			return app.New().Compare(cmd.Context(), algorithms)
		},
	}

	compareCmd.Flags().StringSliceVarP(&algorithmNames, compareAlgorithmFlag, "a", nil,
		fmt.Sprintf("comma-separated algorithms to evaluate (%s); defaults to all",
			strings.Join(algorithm.SearchAlgorithmNames(), ", ")))

	return compareCmd
}
