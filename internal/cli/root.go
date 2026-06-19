package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the snow-white root command. Subcommands are attached by
// AddCommand in main() wiring or follow-up tasks.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "snow-white",
		Short:         "InnovestX market-data collection, analysis, and trading CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}
