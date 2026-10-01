package cli

import (
	"context"
	"strings"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/dnscaleou/dnscale-go/api"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func validZoneSelector(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " /\\\t\r\n?#") || value == "." || value == uuid.Nil.String() {
		return usageError("ZONE must be a nonempty zone UUID or DNS domain name")
	}
	return nil
}
func normalizedZone(value string) string { return strings.ToLower(strings.TrimSuffix(value, ".")) }
func zonePage(ctx context.Context, client *dnscale.Client, offset, limit int) ([]dnscale.Zone, api.Pagination, error) {
	page, err := client.Zones.List(ctx, dnscale.PageOptions{Offset: offset, Limit: limit})
	if err != nil {
		return nil, api.Pagination{}, err
	}
	return page.Zones, page.Pagination, nil
}
func zoneID(zone dnscale.Zone) string {
	if zone.Id == uuid.Nil {
		return ""
	}
	return zone.Id.String()
}

func resolveZone(ctx context.Context, client *dnscale.Client, value string) (uuid.UUID, error) {
	if id, err := uuid.Parse(value); err == nil {
		return id, nil
	}
	zones, _, err := collectPages(ctx, listOptions{limit: 100, all: true}, func(ctx context.Context, offset, limit int) ([]dnscale.Zone, api.Pagination, error) {
		return zonePage(ctx, client, offset, limit)
	}, zoneID)
	if err != nil {
		return uuid.Nil, err
	}
	var found uuid.UUID
	for _, zone := range zones {
		if normalizedZone(zone.Name) != normalizedZone(value) {
			continue
		}
		if found != uuid.Nil {
			return uuid.Nil, resultError("ambiguous_zone", "multiple accessible zones match; use a zone UUID")
		}
		found = zone.Id
	}
	if found == uuid.Nil {
		return uuid.Nil, resultError("zone_not_found", "zone not found among accessible zones")
	}
	return found, nil
}

func (a *app) zonesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "zones", Short: "List, inspect, and create DNS zones"}
	var options listOptions
	list := &cobra.Command{Use: "list", Short: "List accessible zones", Args: noArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := options.validate(100); err != nil {
			return err
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			zones, info, err := collectPages(ctx, options, func(ctx context.Context, offset, limit int) ([]dnscale.Zone, api.Pagination, error) {
				return zonePage(ctx, client, offset, limit)
			}, zoneID)
			if err != nil {
				return err
			}
			return a.printResult(result{Data: zones, Pagination: info})
		})
	}}
	options.flags(list, 100)
	cmd.AddCommand(list)
	cmd.AddCommand(&cobra.Command{Use: "get ZONE", Short: "Get a zone by UUID or exact domain name", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			id, err := resolveZone(ctx, client, args[0])
			if err != nil {
				return err
			}
			zone, err := client.Zones.Get(ctx, id.String())
			if err != nil {
				return err
			}
			return a.print(zone)
		})
	}})
	var region string
	create := &cobra.Command{Use: "create DOMAIN", Short: "Create an active master zone", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		if region != "EU" && region != "GLOBAL" && region != "EU_GLOBAL" {
			return usageError("region must be EU, GLOBAL, or EU_GLOBAL")
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			a.mutation = true
			zone, err := client.Zones.Create(ctx, dnscale.CreateZoneRequest{Name: normalizedZone(args[0]), Region: dnscale.Ptr(api.ZoneRegion(region)), Status: dnscale.Ptr(api.ZoneStatus("active")), Type: dnscale.Ptr(api.ZoneType("master"))})
			if err != nil {
				return err
			}
			return a.print(zone)
		})
	}}
	create.Flags().StringVar(&region, "region", "EU_GLOBAL", "DNS regions: EU, GLOBAL, or EU_GLOBAL")
	cmd.AddCommand(create)
	return cmd
}
