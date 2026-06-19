package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the snow-white root command. Subcommands are attached by
// AddCommand in main() wiring or follow-up tasks.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "snow-white",
		Short:         "InnovestX market-data collection, analysis, and trading CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCollectCmd())
	root.AddCommand(newAnalyzeCmd())
	root.AddCommand(newBacktestCmd())
	root.AddCommand(newTradeCmd())
	root.AddCommand(newOrderCmd())
	root.AddCommand(newBalanceCmd())
	root.AddCommand(newStatusCmd())
	return root
}
