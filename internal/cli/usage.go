package cli

import (
	"context"
	"time"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/dnscaleou/dnscale-go/api"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (a *app) usageCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "usage", Short: "Inspect account and zone usage"}
	for _, operation := range []string{"current", "summary"} {
		var month string
		account := &cobra.Command{Use: operation, Short: "Read " + operation + " account usage (customer-wide key required)", Args: noArgs}
		if operation == "summary" {
			account.Flags().StringVar(&month, "month", "", "Billing month YYYY-MM (default: current month)")
		}
		account.RunE = func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("month") {
				if _, err := time.Parse("2006-01", month); err != nil {
					return usageError("month must use YYYY-MM")
				}
			}
			return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
				var value *api.CustomerUsageResponse
				var body []byte
				if operation == "current" {
					response, err := client.API.GetCurrentUsageWithResponse(ctx)
					if err != nil {
						return err
					}
					value, body = response.JSON200, response.Body
				} else {
					params := &api.GetUsageSummaryParams{}
					if month != "" {
						params.Month = &month
					}
					response, err := client.API.GetUsageSummaryWithResponse(ctx, params)
					if err != nil {
						return err
					}
					value, body = response.JSON200, response.Body
				}
				if err := requireResponseFields(body, "customer_id", "billing_month", "total_queries"); err != nil {
					return err
				}
				if value == nil || value.Data.CustomerId == uuid.Nil || value.Data.BillingMonth.IsZero() {
					return &dnscale.ProtocolError{Message: "invalid account usage response"}
				}
				return a.print(value.Data)
			})
		}
		cmd.AddCommand(account)
	}
	var start, end string
	zone := &cobra.Command{Use: "zone ZONE", Short: "Read zone usage (full-zone access required)", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		params := &api.GetZoneUsageParams{}
		if cmd.Flags().Changed("start-date") || cmd.Flags().Changed("end-date") {
			from, err := time.Parse("2006-01-02", start)
			if err != nil {
				return usageError("supply both --start-date and --end-date using YYYY-MM-DD")
			}
			to, err := time.Parse("2006-01-02", end)
			if err != nil || to.Before(from) {
				return usageError("end-date must use YYYY-MM-DD and be on or after start-date")
			}
			params.StartDate = &api.StartDate{Time: from}
			params.EndDate = &api.EndDate{Time: to}
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			id, err := resolveZone(ctx, client, args[0])
			if err != nil {
				return err
			}
			response, err := client.API.GetZoneUsageWithResponse(ctx, id, params)
			if err != nil {
				return err
			}
			if err := requireResponseFields(response.Body, "zone_name", "period_start", "period_end", "total_queries"); err != nil {
				return err
			}
			if response.JSON200 == nil || response.JSON200.Data.ZoneName == "" || response.JSON200.Data.PeriodStart.IsZero() || response.JSON200.Data.PeriodEnd.IsZero() {
				return &dnscale.ProtocolError{Message: "invalid zone usage response"}
			}
			return a.print(response.JSON200.Data)
		})
	}}
	zone.Flags().StringVar(&start, "start-date", "", "Inclusive start date YYYY-MM-DD (supply both dates or neither)")
	zone.Flags().StringVar(&end, "end-date", "", "Inclusive end date YYYY-MM-DD (server defaults to today)")
	cmd.AddCommand(zone)
	return cmd
}
