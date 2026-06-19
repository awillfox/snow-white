package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/order"
)

func newKillCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "kill",
		Short: "Halt live trading immediately (sets risk_state.halted=true for today)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			store := order.NewStore(pool)
			if err := store.SetHalted(ctx, time.Now(), true, reason); err != nil {
				return fmt.Errorf("set halted: %w", err)
			}
			fmt.Printf("trading halted: reason=%q\n", reason)
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "reason for halting (recorded in risk_state)")
	return cmd
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Clear trading halt (prompts for confirmation before resuming)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Print("Resume trading? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			line = strings.TrimSpace(line)
			if line != "y" && line != "Y" {
				fmt.Println("aborted")
				return nil
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			store := order.NewStore(pool)
			if err := store.SetHalted(ctx, time.Now(), false, ""); err != nil {
				return fmt.Errorf("clear halt: %w", err)
			}
			fmt.Println("trading resumed")
			return nil
		},
	}
}
