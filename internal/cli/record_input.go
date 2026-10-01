package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	dnscale "github.com/dnscaleou/dnscale-go"
	"github.com/dnscaleou/dnscale-go/api"
	"github.com/oapi-codegen/nullable"
	"github.com/spf13/cobra"
)

const maxRecordInput = 1 << 20

type recordInput struct {
	file, name, typ, content, comment string
	ttl, priority                     int
	disabled, dryRun                  bool
}

func (o *recordInput) flags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&o.file, "file", "", "Read one API-shaped JSON object from PATH or - for stdin (maximum 1 MiB)")
	f.StringVar(&o.name, "name", "", "Relative name, @ for the apex, or fully qualified name")
	f.StringVar(&o.typ, "type", "", "DNS record type, for example A, AAAA, TXT, CNAME, or MX")
	f.StringVar(&o.content, "content", "", "Record value (MX target uses --priority; full SRV content includes priority)")
	f.IntVar(&o.ttl, "ttl", 0, "RRset TTL in seconds; documented range 300–86400, omitted/0 uses server default")
	f.IntVar(&o.priority, "priority", 0, "Priority, including an explicit zero")
	f.BoolVar(&o.disabled, "disabled", false, "Disable this value; omitted defaults to false")
	f.StringVar(&o.comment, "comment", "", "DNScale RRset comment; empty clears it, omitted preserves it")
	f.BoolVar(&o.dryRun, "dry-run", false, "Validate input offline without credentials or API requests")
}
func (o recordInput) read(a *app, cmd *cobra.Command) (dnscale.CreateRecordRequest, error) {
	var body dnscale.CreateRecordRequest
	if cmd.Flags().Changed("file") {
		if o.file == "" {
			return body, usageError("file must be a path or -")
		}
		for _, name := range []string{"name", "type", "content", "ttl", "priority", "disabled", "comment"} {
			if cmd.Flags().Changed(name) {
				return body, usageError("--file cannot be combined with record field flags")
			}
		}
		reader := a.in
		if o.file != "-" {
			f, err := os.Open(o.file)
			if err != nil {
				return body, usageError("cannot open record input file")
			}
			defer f.Close()
			reader = f
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxRecordInput+1))
		if err != nil {
			return body, usageError("cannot read record input")
		}
		if len(data) > maxRecordInput {
			return body, usageError("record input exceeds 1 MiB")
		}
		if !utf8.Valid(data) {
			return body, usageError("record input must be UTF-8 JSON")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			return body, usageError("invalid record JSON: " + err.Error())
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return body, usageError("record input must contain exactly one JSON object")
		}
	} else {
		body = dnscale.CreateRecordRequest{Name: o.name, Type: api.RecordType(o.typ), Content: o.content}
		if cmd.Flags().Changed("ttl") {
			body.Ttl = dnscale.Ptr(o.ttl)
		}
		if cmd.Flags().Changed("priority") {
			body.Priority = nullable.NewNullableWithValue(o.priority)
		}
		if cmd.Flags().Changed("disabled") {
			body.Disabled = dnscale.Ptr(o.disabled)
		}
		if cmd.Flags().Changed("comment") {
			body.Comment = nullable.NewNullableWithValue(o.comment)
		}
	}
	body.Type = api.RecordType(strings.ToUpper(string(body.Type)))
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Content) == "" || body.Type == "" {
		return body, usageError("name, type, and content are required")
	}
	if strings.ContainsAny(body.Name, " \t\r\n") {
		return body, usageError("record name cannot contain whitespace")
	}
	switch body.Type {
	case api.A, api.AAAA, api.ALIAS, api.CAA, api.CNAME, api.HTTPS, api.MX, api.NAPTR, api.NS, api.PTR, api.SOA, api.SRV, api.SVCB, api.TLSA, api.TXT:
	default:
		return body, usageError("unsupported record type in the published API contract")
	}
	if body.Type == api.TXT && strings.TrimSpace(body.Content) != body.Content {
		return body, usageError("TXT content cannot have leading or trailing whitespace")
	}
	if body.Ttl != nil && *body.Ttl != 0 && (*body.Ttl < 120 || *body.Ttl > 86400) {
		return body, usageError("TTL is outside the accepted API range; use the documented 300–86400 seconds or 0 for the default")
	}
	if comment, err := body.Comment.Get(); err == nil && utf8.RuneCountInString(strings.TrimSpace(comment)) > 500 {
		return body, usageError("comment must be at most 500 characters")
	}
	if !utf8.ValidString(body.Name) || !utf8.ValidString(body.Content) {
		return body, usageError("record fields must be valid UTF-8")
	}
	return body, nil
}

func recordDryRun(a *app, operation, zone, id string, body dnscale.CreateRecordRequest) error {
	return a.print(map[string]any{"valid": true, "submitted": false, "validation": "local_only", "operation": operation, "zone": zone, "record_id": id, "record": body})
}

func validRecordID(id string) error {
	if strings.TrimSpace(id) == "" {
		return usageError("RECORD_ID must be a nonempty opaque ID from the API")
	}
	return nil
}
