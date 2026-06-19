package cli

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/invx"
	"snow-white/internal/session"
	"snow-white/pkg/scale"
)

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Record a trading session start or end snapshot (combined net worth)",
	}
	cmd.AddCommand(newSessionStartCmd())
	cmd.AddCommand(newSessionEndCmd())
	return cmd
}

func newSessionStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Snapshot combined net worth and record a session_start event",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionEvent(cmd, 1)
		},
	}
}

func newSessionEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "end",
		Short: "Snapshot combined net worth and record a session_end event",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionEvent(cmd, 2)
		},
	}
}

func runSessionEvent(cmd *cobra.Command, event int32) error {
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

	client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
	store := session.NewStore(pool)
	tracker := session.NewTracker(client, store)

	bal, err := tracker.CombinedBalanceSatang(ctx)
	if err != nil {
		return err
	}
	if err := store.InsertSessionTrack(ctx, event, bal); err != nil {
		return err
	}

	eventName := "start"
	if event == 2 {
		eventName = "end"
	}
	fmt.Printf("session %s recorded: combined_balance=%s THB\n",
		eventName,
		scale.Format(bal, 2),
	)
	return nil
}
