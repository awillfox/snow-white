package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/discord"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/trader"
	"snow-white/pkg/scale"
)

func newOrderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "order",
		Short: "Manual order management (send, cancel, open, hist)",
	}
	cmd.AddCommand(newOrderSendCmd())
	cmd.AddCommand(newOrderCancelCmd())
	cmd.AddCommand(newOrderOpenCmd())
	cmd.AddCommand(newOrderHistCmd())
	return cmd
}

func newOrderSendCmd() *cobra.Command {
	var symbol, sideStr, typeStr, priceStr, qtyStr, valueStr string
	var live bool

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a manual order (requires --live to place; dry-run by default)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			if sideStr == "" {
				return fmt.Errorf("--side required (BUY or SELL)")
			}
			if qtyStr == "" && valueStr == "" {
				return fmt.Errorf("exactly one of --qty or --value required")
			}
			if qtyStr != "" && valueStr != "" {
				return fmt.Errorf("specify only one of --qty or --value, not both")
			}

			// Parse side
			var side invx.Side
			switch strings.ToUpper(sideStr) {
			case "BUY":
				side = invx.Buy
			case "SELL":
				side = invx.Sell
			default:
				return fmt.Errorf("--side must be BUY or SELL, got %q", sideStr)
			}

			// Parse order type (default LIMIT; flag default ensures typeStr is never empty)
			orderType := invx.Limit
			switch strings.ToUpper(typeStr) {
			case "LIMIT":
				orderType = invx.Limit
			case "MARKET":
				orderType = invx.Market
			default:
				return fmt.Errorf("--type must be LIMIT or MARKET, got %q", typeStr)
			}

			// Parse price (required for limit orders)
			var priceSatang int64
			if priceStr != "" {
				p, err := scale.Parse(priceStr, 2)
				if err != nil {
					return fmt.Errorf("--price: %w", err)
				}
				priceSatang = p
			} else if orderType == invx.Limit {
				return fmt.Errorf("--price required for LIMIT orders")
			}

			// Parse qty or value
			var qtyUnits, valueSatang int64
			if qtyStr != "" {
				q, err := scale.Parse(qtyStr, 8)
				if err != nil {
					return fmt.Errorf("--qty: %w", err)
				}
				qtyUnits = q
			}
			if valueStr != "" {
				v, err := scale.Parse(valueStr, 2)
				if err != nil {
					return fmt.Errorf("--value: %w", err)
				}
				valueSatang = v
			}

			// Guard: reject zero-size orders (catches --value 0 and scale.Parse silent-zero).
			if qtyUnits == 0 && valueSatang == 0 {
				return fmt.Errorf("order size is zero: provide a non-zero --qty or --value")
			}

			// The API misinterprets value-based limit orders (sends wildly wrong origQty).
			// Convert --value to quantity using the limit price so we always send Quantity.
			if valueSatang > 0 && qtyUnits == 0 {
				if priceSatang <= 0 {
					return fmt.Errorf("--value requires --price (to convert to quantity)")
				}
				qtyUnits = valueSatang * 1e8 / priceSatang
				valueSatang = 0
			}

			in := invx.SendOrderInput{
				Symbol:     symbol,
				Side:       side,
				Type:       orderType,
				LimitPrice: priceSatang,
				Quantity:   qtyUnits,
				Value:      valueSatang,
				// InnovestX requires clientOrderId > 0 (despite the docs marking it
				// "optional"). Use a millisecond timestamp — positive and unique
				// enough for manual orders; cancel by --order-id otherwise.
				ClientOrderID: time.Now().UnixMilli(),
			}

			sideLabel := "BUY"
			if side == invx.Sell {
				sideLabel = "SELL"
			}
			typeLabel := "LIMIT"
			if orderType == invx.Market {
				typeLabel = "MARKET"
			}
			fmt.Printf("order: symbol=%s side=%s type=%s price=%s qty=%s value=%s\n",
				in.Symbol,
				sideLabel,
				typeLabel,
				scale.Format(in.LimitPrice, 2),
				scale.Format(in.Quantity, 8),
				scale.Format(in.Value, 2),
			)

			if !live {
				fmt.Println("DRY-RUN — would send the above order (pass --live to place)")
				return nil
			}

			// Confirm before placing a live order
			fmt.Print("Send LIVE order? [y/N]: ")
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

			// Kill-switch check 1: file-based halt.
			if trader.KillFileTripped(cfg.KillFile) {
				return fmt.Errorf("blocked: kill file present (%s)", cfg.KillFile)
			}

			// Kill-switch check 2: DB halt flag.
			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()
			state, err := order.NewStore(pool).RiskToday(ctx, time.Now())
			if err != nil {
				return fmt.Errorf("risk state: %w", err)
			}
			if state.Halted {
				return fmt.Errorf("blocked: trading halted (resume to clear)")
			}

			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			orderID, err := client.SendOrder(ctx, in)
			if err != nil {
				return err
			}
			fmt.Printf("order placed: orderId=%d\n", orderID)

			// Non-fatal Discord notification — intentionally synchronous: this is a
			// one-shot CLI process that exits immediately after; a goroutine would be
			// killed before it delivers the message.
			dc := discord.New(cfg.DiscordWebhookURL)
			notifyMsg := fmt.Sprintf("📝 manual LIVE %s %s qty=%s price=%s (orderId=%d)",
				sideLabel,
				symbol,
				scale.Format(in.Quantity, 8),
				scale.Format(in.LimitPrice, 2),
				orderID,
			)
			if err := dc.Send(ctx, notifyMsg); err != nil {
				log.Printf("order: discord notify error (non-fatal): %v", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&symbol, "symbol", "", "trading symbol, e.g. BTCTHB")
	cmd.Flags().StringVar(&sideStr, "side", "", "BUY or SELL")
	cmd.Flags().StringVar(&typeStr, "type", "LIMIT", "LIMIT or MARKET (default LIMIT)")
	cmd.Flags().StringVar(&priceStr, "price", "", "limit price in THB (required for LIMIT orders)")
	cmd.Flags().StringVar(&qtyStr, "qty", "", "coin quantity (e.g. 0.001)")
	cmd.Flags().StringVar(&valueStr, "value", "", "THB value to spend (e.g. 1000)")
	cmd.Flags().BoolVar(&live, "live", false, "place REAL order (default: dry-run, prints intent only)")
	return cmd
}

func newOrderCancelCmd() *cobra.Command {
	var orderID, clientOrderID int64

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel an open order by order-id or client-order-id",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if orderID == 0 && clientOrderID == 0 {
				return fmt.Errorf("one of --order-id or --client-order-id required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			if err := client.CancelOrder(ctx, clientOrderID, orderID); err != nil {
				return err
			}
			fmt.Println("canceled")
			return nil
		},
	}

	cmd.Flags().Int64Var(&orderID, "order-id", 0, "exchange order ID to cancel")
	cmd.Flags().Int64Var(&clientOrderID, "client-order-id", 0, "client order ID to cancel")
	return cmd
}

func newOrderOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "List open orders",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			orders, err := client.OpenOrders(ctx)
			if err != nil {
				return err
			}
			if len(orders) == 0 {
				fmt.Println("no open orders")
				return nil
			}
			for _, o := range orders {
				printOrderInfo(o)
			}
			return nil
		},
	}
}

func newOrderHistCmd() *cobra.Command {
	var symbol string
	var depth int

	cmd := &cobra.Command{
		Use:   "hist",
		Short: "List order history for a symbol",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			orders, err := client.OrderHistory(ctx, symbol, depth)
			if err != nil {
				return err
			}
			if len(orders) == 0 {
				fmt.Println("no order history")
				return nil
			}
			for _, o := range orders {
				printOrderInfo(o)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&symbol, "symbol", "", "trading symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&depth, "depth", 200, "number of historical orders to fetch")
	return cmd
}

func printOrderInfo(o invx.OrderInfo) {
	sideLabel := "BUY"
	if o.Side == invx.Sell {
		sideLabel = "SELL"
	}
	typeLabel := "LIMIT"
	if o.Type == invx.Market {
		typeLabel = "MARKET"
	}
	fmt.Printf("orderId=%d clientOrderId=%d symbol=%s side=%s type=%s state=%s price=%s origQty=%s\n",
		o.OrderID,
		o.ClientOrderID,
		o.Symbol,
		sideLabel,
		typeLabel,
		o.State,
		scale.Format(o.Price, 2),
		scale.Format(o.OrigQuantity, 8),
	)
}

// newBalanceCmd fetches and prints account balances.
func newBalanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show account balances",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			balances, err := client.AccountBalance(ctx)
			if err != nil {
				return err
			}
			if len(balances) == 0 {
				fmt.Println("no balances returned")
				return nil
			}
			for _, b := range balances {
				if b.Amount == 0 && b.Hold == 0 {
					continue // skip zero-balance rows
				}
				fmt.Printf("product=%s amount=%s hold=%s\n",
					b.Product,
					scale.Format(b.Amount, 8),
					scale.Format(b.Hold, 8),
				)
			}
			return nil
		},
	}
}

// newStatusCmd prints today's risk state and position from PG.
func newStatusCmd() *cobra.Command {
	var symbol string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show today's risk state and position from the database",
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
			risk, err := store.RiskToday(ctx, time.Now())
			if err != nil {
				return fmt.Errorf("risk state: %w", err)
			}
			fmt.Printf("day=%s halted=%v halt_reason=%q spent_today=%s loss_today=%s\n",
				risk.Day.Format("2006-01-02"),
				risk.Halted,
				risk.HaltReason,
				scale.Format(risk.SpentToday, 2),
				scale.Format(risk.LossToday, 2),
			)

			if symbol != "" {
				pos, err := store.GetPosition(ctx, symbol)
				if err != nil {
					return fmt.Errorf("get position: %w", err)
				}
				fmt.Printf("symbol=%s qty=%s avg_cost=%s realized_pnl=%s\n",
					pos.Symbol,
					scale.Format(pos.Qty, 8),
					scale.Format(pos.AvgCost, 2),
					scale.Format(pos.RealizedPnl, 2),
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol to show position for, e.g. BTCTHB")
	return cmd
}
