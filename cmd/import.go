package cmd

import (
	"github.com/ONSdigital/dis-search-test-bed/app"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const importCommandName = "import"

func importCommand() (*cobra.Command, error) {
	var inputPath string

	importCmd := &cobra.Command{
		Use:   importCommandName,
		Short: "Import data",
		Args:  cobra.NoArgs,
	}
	importCmd.PersistentFlags().StringVarP(&inputPath, "input", "i", "", "path to the CSV input file")
	if err := importCmd.MarkPersistentFlagRequired("input"); err != nil {
		return nil, errors.Wrap(err, "failed to require input flag")
	}
	importCmd.AddCommand(importJudgementsCommand(&inputPath))

	return importCmd, nil
}

func importJudgementsCommand(inputPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "judgements",
		Short: "Import re-scored relevance judgements from an export-format CSV",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.New().ImportJudgements(cmd.Context(), *inputPath)
		},
	}
}
