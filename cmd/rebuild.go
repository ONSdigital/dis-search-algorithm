package cmd

import (
	"github.com/ONSdigital/dis-search-algorithm/app"
	"github.com/spf13/cobra"
)

const rebuildCommandName = "rebuild"

func rebuildCommand() (*cobra.Command, error) {
	var outputPath string

	exportCmd := &cobra.Command{
		Use:   rebuildCommandName,
		Short: "Rebuild data",
		Args:  cobra.NoArgs,
	}
	exportCmd.AddCommand(rebuildJudgementsCommand(&outputPath))

	return exportCmd, nil
}

func rebuildJudgementsCommand(outputPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "judgements",
		Short: "Rebuild ranked items and their current relevance judgements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.New().RebuildAllTermJudgements(cmd.Context())
		},
	}
}
