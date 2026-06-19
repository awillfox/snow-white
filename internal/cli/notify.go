package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/discord"
)

func newNotifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "notify <message...>",
		Short: "Send a message to Discord (DISCORD_BOT_URL must be set)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.DiscordWebhookURL == "" {
				return fmt.Errorf("DISCORD_BOT_URL not set")
			}
			message := strings.Join(args, " ")
			dc := discord.New(cfg.DiscordWebhookURL)
			if err := dc.Send(cmd.Context(), message); err != nil {
				return fmt.Errorf("discord notify: %w", err)
			}
			fmt.Println("sent")
			return nil
		},
	}
}
