package cli

import (
	"context"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/dnscaleou/dnscale-go/api"
	"github.com/spf13/cobra"
)

func (a *app) recordsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "records", Short: "List, inspect, create, update, and delete individual DNS values"}
	var options listOptions
	list := &cobra.Command{Use: "list ZONE", Short: "List records in an accessible zone", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := options.validate(1000); err != nil {
			return err
		}
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			id, err := resolveZone(ctx, client, args[0])
			if err != nil {
				return err
			}
			records, info, err := collectPages(ctx, options, func(ctx context.Context, offset, limit int) ([]dnscale.Record, api.Pagination, error) {
				page, err := client.Records.List(ctx, id.String(), dnscale.PageOptions{Offset: offset, Limit: limit})
				if err != nil {
					return nil, api.Pagination{}, err
				}
				return page.Records, page.Pagination, nil
			}, func(record dnscale.Record) string { return record.Id })
			if err != nil {
				return err
			}
			return a.printResult(result{Data: records, Pagination: info})
		})
	}}
	options.flags(list, 1000)
	cmd.AddCommand(list)
	cmd.AddCommand(&cobra.Command{Use: "get ZONE RECORD_ID", Short: "Get one value using its opaque API ID", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		if err := validRecordID(args[1]); err != nil {
			return err
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			id, err := resolveZone(ctx, client, args[0])
			if err != nil {
				return err
			}
			record, err := client.Records.Get(ctx, id.String(), args[1])
			if err != nil {
				return err
			}
			return a.print(record)
		})
	}})
	for _, operation := range []string{"create", "update"} {
		var input recordInput
		use, n := operation+" ZONE", 1
		if operation == "update" {
			use += " RECORD_ID"
			n = 2
		}
		write := &cobra.Command{Use: use, Short: operation + " a DNS value using a complete record body", Long: "Supply name, type, and content using field flags or --file. Update follows PUT semantics: omitted TTL defaults to 3600 and disabled to false; omitted/null comment preserves it. TTL and comments apply to the RRset. Retain the returned ID after an update.", Args: exactArgs(n)}
		input.flags(write)
		write.RunE = func(cmd *cobra.Command, args []string) error {
			if err := validZoneSelector(args[0]); err != nil {
				return err
			}
			recordID := ""
			if operation == "update" {
				recordID = args[1]
				if err := validRecordID(recordID); err != nil {
					return err
				}
			}
			body, err := input.read(a, cmd)
			if err != nil {
				return err
			}
			if input.dryRun {
				return recordDryRun(a, operation, args[0], recordID, body)
			}
			return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
				id, err := resolveZone(ctx, client, args[0])
				if err != nil {
					return err
				}
				a.mutation = true
				var record *dnscale.Record
				if operation == "create" {
					record, err = client.Records.Create(ctx, id.String(), body)
				} else {
					record, err = client.Records.Update(ctx, id.String(), recordID, body)
				}
				if err != nil {
					return err
				}
				return a.print(record)
			})
		}
		cmd.AddCommand(write)
	}
	var yes bool
	remove := &cobra.Command{Use: "delete ZONE RECORD_ID", Short: "Delete the selected DNS value; requires --yes", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return usageError("record deletion requires --yes")
		}
		if err := validZoneSelector(args[0]); err != nil {
			return err
		}
		if err := validRecordID(args[1]); err != nil {
			return err
		}
		return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
			id, err := resolveZone(ctx, client, args[0])
			if err != nil {
				return err
			}
			a.mutation = true
			if err := client.Records.Delete(ctx, id.String(), args[1]); err != nil {
				return err
			}
			return a.print(map[string]any{"deleted": true, "zone_id": id, "record_id": args[1]})
		})
	}}
	remove.Flags().BoolVar(&yes, "yes", false, "Acknowledge deletion of this record")
	cmd.AddCommand(remove)
	return cmd
}
