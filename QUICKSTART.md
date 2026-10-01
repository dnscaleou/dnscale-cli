# First DNS workflow

Build the client with `make build` and put `bin` on your PATH.
These commands make real DNS changes when submitted. Use a disposable sandbox
and a key with zone/record read/write,
DNSSEC read, and usage read scopes. Replace the example endpoint and domain
with your test environment and an appropriate test zone.

Save the key from `DNSCALE_API_KEY` into a profile:

```sh
dnscale auth login --profile sandbox --base-url https://YOUR_SANDBOX_HOST/v1
dnscale auth status --profile sandbox
dnscale zones create YOUR_TEST_ZONE --region EU --profile sandbox
```

The returned `data.id` is the zone UUID. Use it as `ZONE_ID` below to avoid name
resolution requests. Create a record file:

```json
{"name":"_cli","type":"TXT","content":"first-value","ttl":300}
```

Validate it locally, submit it, and add a sibling at the same owner:

```sh
dnscale records create ZONE_ID --file record.json --dry-run
dnscale records create ZONE_ID --file record.json --profile sandbox --json
dnscale records create ZONE_ID --name _cli --type TXT --content sibling-value --ttl 300 --profile sandbox
dnscale records list ZONE_ID --limit 1 --all --profile sandbox --json
```

Keep the first creation response's `data.id` as `RECORD_ID`. IDs are opaque;
do not construct them from a name or decode them in scripts. Replace only the
selected value:

```sh
dnscale records get ZONE_ID RECORD_ID --profile sandbox
dnscale records update ZONE_ID RECORD_ID --name _cli --type TXT --content replacement-value --ttl 300 --profile sandbox --json
```

Retain this response's `data.id` as `UPDATED_RECORD_ID`. This is a complete PUT
body: optional fields use the defaults described in the reference. The API
keeps the sibling value. TTL and comment changes affect their shared RRset.

```sh
dnscale records get ZONE_ID UPDATED_RECORD_ID --profile sandbox
dnscale records delete ZONE_ID UPDATED_RECORD_ID --yes --profile sandbox
dnscale records list ZONE_ID --all --profile sandbox
dnscale dnssec status ZONE_ID --profile sandbox
dnscale dnssec ds ZONE_ID --profile sandbox
dnscale usage current --profile sandbox
```

Confirm `sibling-value` remains. Remove that value by its own ID and remove the
disposable zone through the dashboard when finished. The CLI does not expose
whole-RRset or zone deletion. DNSSEC status and DS inspection are read-only;
registrar delegation is a separate step.

For scripts and terminal agents, pass `--json`, inspect the exit code, read
`data.id` after writes, and supply `--yes` explicitly for deletions. Use stdin
with `--file -` for generated requests. Treat timeout/network failures as an
uncertain result and inspect state before repeating a write. Never put a key
in command arguments or commit it in a request file.
