package cli

import (
	"context"
	"encoding/json"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/spf13/cobra"
)

func requireResponseFields(body []byte, fields ...string) error {
	var envelope struct {
		Status string
		Data   map[string]json.RawMessage
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Status != "success" || envelope.Data == nil {
		return &dnscale.ProtocolError{Message: "missing success data object"}
	}
	for _, field := range fields {
		value, ok := envelope.Data[field]
		if !ok || string(value) == "null" {
			return &dnscale.ProtocolError{Message: "missing response field: " + field}
		}
	}
	return nil
}

func (a *app) dnssecCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "dnssec", Short: "Inspect DNSSEC status and public DS records (requires full-zone access)"}
	for _, operation := range []string{"status", "ds"} {
		cmd.AddCommand(&cobra.Command{Use: operation + " ZONE", Short: map[string]string{"status": "Read signing status; disabled is a successful result", "ds": "Read public DS records for the registrar"}[operation], Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if err := validZoneSelector(args[0]); err != nil {
				return err
			}
			return a.withClient(cmd, func(ctx context.Context, client *dnscale.Client) error {
				id, err := resolveZone(ctx, client, args[0])
				if err != nil {
					return err
				}
				if operation == "status" {
					response, err := client.API.GetDNSSECStatusWithResponse(ctx, id)
					if err != nil {
						return err
					}
					if err := requireResponseFields(response.Body, "dnssec"); err != nil {
						return err
					}
					if response.JSON200 == nil {
						return &dnscale.ProtocolError{Message: "invalid DNSSEC status response"}
					}
					return a.print(response.JSON200.Data)
				}
				response, err := client.API.ListDNSSECDSRecordsWithResponse(ctx, id)
				if err != nil {
					return err
				}
				if err := requireResponseFields(response.Body); err != nil {
					return err
				}
				// The current handler emits null when the zone has no signing keys.
				// Accept that empty collection, but reject a missing field.
				var envelope struct{ Data map[string]json.RawMessage }
				_ = json.Unmarshal(response.Body, &envelope)
				if _, ok := envelope.Data["ds_records"]; !ok {
					return &dnscale.ProtocolError{Message: "missing response field: ds_records"}
				}
				if response.JSON200 == nil {
					return &dnscale.ProtocolError{Message: "invalid DS response"}
				}
				if response.JSON200.Data.DsRecords == nil {
					response.JSON200.Data.DsRecords = []string{}
				}
				return a.print(response.JSON200.Data)
			})
		}})
	}
	return cmd
}
