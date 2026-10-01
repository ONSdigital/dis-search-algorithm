package cmd

import (
	"github.com/ONSdigital/dis-search-test-bed/app"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const exportCommandName = "export"

func exportCommand() (*cobra.Command, error) {
	var outputPath string

	exportCmd := &cobra.Command{
		Use:   exportCommandName,
		Short: "Export data",
		Args:  cobra.NoArgs,
	}
	exportCmd.PersistentFlags().StringVarP(&outputPath, "output", "o", "", "path to the CSV output file")
	if err := exportCmd.MarkPersistentFlagRequired("output"); err != nil {
		return nil, errors.Wrap(err, "failed to require output flag")
	}
	exportCmd.AddCommand(exportJudgementsCommand(&outputPath))

	return exportCmd, nil
}

func exportJudgementsCommand(outputPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "judgements",
		Short: "Export ranked items and their current relevance judgements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.New().ExportJudgements(cmd.Context(), *outputPath)
		},
	}
}
