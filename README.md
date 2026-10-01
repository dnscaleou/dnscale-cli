# DNScale CLI

Manage DNS zones and records, inspect DNSSEC, and read usage from scripts or a
terminal. The standalone Go module uses the public DNScale Go SDK.

**Current release: `1.0.0`.** Start with read-only commands and offline
validation, then test record changes in a disposable zone before using them in
production.

## Repositories and releases

| Resource | Link |
| --- | --- |
| CLI source and issues | [dnscaleou/dnscale-cli](https://github.com/dnscaleou/dnscale-cli) |
| Version 1.0.0 downloads | [Release archives and notes](https://github.com/dnscaleou/dnscale-cli/releases/tag/v1.0.0) |
| All releases | [Release history](https://github.com/dnscaleou/dnscale-cli/releases) |
| Homebrew package | [dnscaleou/homebrew-tap](https://github.com/dnscaleou/homebrew-tap) |
| Homebrew formula | [Formula/dnscale.rb](https://github.com/dnscaleou/homebrew-tap/blob/main/Formula/dnscale.rb) |

## Install

### Homebrew (macOS and Linux)

```sh
brew install dnscaleou/tap/dnscale
dnscale --version
```

The [official tap](https://github.com/dnscaleou/homebrew-tap) selects your
platform's release archive, verifies its checksum, and installs Bash, Zsh, and
Fish completions. Run `brew update` followed by `brew upgrade dnscale` to
upgrade, or `brew uninstall dnscale` to remove it.

### Binary downloads

Download your platform's archive and [checksums.txt](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/checksums.txt) from the
[1.0.0 release](https://github.com/dnscaleou/dnscale-cli/releases/tag/v1.0.0).
Prebuilt binaries do not require Go.

| Platform | Archive |
| --- | --- |
| macOS Apple Silicon | [dnscale_1.0.0_darwin_arm64.tar.gz](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/dnscale_1.0.0_darwin_arm64.tar.gz) |
| macOS Intel | [dnscale_1.0.0_darwin_amd64.tar.gz](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/dnscale_1.0.0_darwin_amd64.tar.gz) |
| Linux ARM64 | [dnscale_1.0.0_linux_arm64.tar.gz](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/dnscale_1.0.0_linux_arm64.tar.gz) |
| Linux AMD64 | [dnscale_1.0.0_linux_amd64.tar.gz](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/dnscale_1.0.0_linux_amd64.tar.gz) |
| Windows AMD64 | [dnscale_1.0.0_windows_amd64.zip](https://github.com/dnscaleou/dnscale-cli/releases/download/v1.0.0/dnscale_1.0.0_windows_amd64.zip) |

On macOS or Linux, compare the SHA-256 hash with the matching line in
`checksums.txt` before extracting. For example, on Apple Silicon:

```sh
shasum -a 256 dnscale_1.0.0_darwin_arm64.tar.gz
tar -xzf dnscale_1.0.0_darwin_arm64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 755 dnscale "$HOME/.local/bin/dnscale"
export PATH="$HOME/.local/bin:$PATH"
dnscale --version
```

Use the archive name for your platform. Linux also provides `sha256sum` for
verification. Add the PATH setting to your shell startup file to keep it
across sessions. Archives include the quickstart and example record files.

On Windows, compare the hash with `checksums.txt`, then extract the archive
and add the destination directory to your user PATH:

```powershell
Get-FileHash .\dnscale_1.0.0_windows_amd64.zip -Algorithm SHA256
Expand-Archive .\dnscale_1.0.0_windows_amd64.zip -DestinationPath "$env:LOCALAPPDATA\DNScale\bin" -Force
& "$env:LOCALAPPDATA\DNScale\bin\dnscale.exe" --version
```

### Install with Go

With Go 1.25 or newer:

```sh
go install github.com/dnscaleou/dnscale-cli/cmd/dnscale@v1.0.0
```

Add `GOBIN`, or the `bin` directory under `go env GOPATH` when `GOBIN` is unset,
to your PATH.

### Build from source

```sh
git clone --branch v1.0.0 https://github.com/dnscaleou/dnscale-cli.git
cd dnscale-cli
make build
bin/dnscale --version
bin/dnscale --help
```

Build/check/release targets pin Go 1.25.14 using `.go-version` and Go's toolchain
selection. On Windows, use `go build -o bin/dnscale.exe ./cmd/dnscale`.

### Upgrade, remove, and complete commands

For manual installs, download and verify the new archive, then replace the
binary. Go users can install the desired version tag or use `@latest`.
Remove the installed binary and completion files to uninstall. Run
`dnscale auth logout --profile NAME` for each saved profile first if you also
want to remove its keychain credential; revoke API keys separately in the
dashboard.

Generate completion with `dnscale completion bash`, `zsh`, `fish`, or
`powershell`; its help describes installation for each shell. Homebrew installs
Bash, Zsh, and Fish completions automatically.

## Authentication

Create a scoped API key in the DNScale dashboard. Set `DNSCALE_API_KEY` from
your secret manager or CI secret. The CLI never reads `.env` files and never
accepts a key as a command-line flag.

```sh
dnscale auth status
dnscale auth login --profile work       # stores DNSCALE_API_KEY in OS keychain
dnscale auth status --profile work
dnscale auth profiles
dnscale auth use work
dnscale auth logout --profile work     # local removal; does not revoke the key
```

`auth login --key-stdin` reads a token from stdin instead of the environment.
It never prompts. Keychain failure returns an error; there is no plaintext
fallback. Environment authentication works in CI without a keychain.

Credentials are bound to the endpoint saved with a profile. For a provisioned
sandbox, save a separate profile with its actual `/v1` URL:

```sh
dnscale auth login --profile sandbox --base-url https://YOUR_SANDBOX_HOST/v1
```

The endpoint defaults to `https://api.dnscale.eu/v1`. A profile named `sandbox`
does not simulate DNS writes. `auth status` reports local configuration only;
it does not validate a key or infer account identity.

Selection order:

1. `--profile`, then `DNSCALE_PROFILE`: use only that stored profile and ignore
   ambient `DNSCALE_API_KEY` and `DNSCALE_BASE_URL`.
2. Otherwise, `DNSCALE_API_KEY` uses `--base-url`, then `DNSCALE_BASE_URL`, then
   the default endpoint.
3. Otherwise, use the default saved profile. An incompatible `--base-url` is
   rejected for stored credentials.

Metadata lives in `<os.UserConfigDir()>/dnscale/config.json`, or
`$DNSCALE_CONFIG_DIR/config.json`, with atomic file replacement and restrictive
permissions. Keys live in OS keychain service `eu.dnscale.cli` under IDs derived
from the profile and endpoint. Profiles contain no tokens.

## Commands

| Command | Purpose |
| --- | --- |
| `zones list [--offset N --limit N --all]` | Accessible zones; limit 1–100 |
| `zones get ZONE` | Zone details |
| `zones create DOMAIN [--region EU\|GLOBAL\|EU_GLOBAL]` | Active master zone; region defaults to EU_GLOBAL |
| `records list ZONE [--offset N --limit N --all]` | Records; limit 1–1000 |
| `records get ZONE RECORD_ID` | One record value |
| `records create ZONE` | Create a value using record flags or a file |
| `records update ZONE RECORD_ID` | Replace the selected value using a complete body |
| `records delete ZONE RECORD_ID --yes` | Delete the selected value |
| `dnssec status ZONE` | Signing status; disabled is a successful result |
| `dnssec ds ZONE` | Public DS records; an empty result is `[]` |
| `usage current` | Current billing-month usage |
| `usage summary [--month YYYY-MM]` | Selected month; defaults to current |
| `usage zone ZONE [--start-date DATE --end-date DATE]` | Daily zone usage; supply both dates or neither |
| `completion bash\|zsh\|fish\|powershell` | Generate shell completion |

`ZONE` accepts a UUID or an exact domain name, ignoring case and a terminal dot.
Name resolution scans accessible zones with pagination and rejects ambiguity.
IDs go directly to the endpoint. Record IDs remain opaque: retain the ID
returned after an update. Zone deletion, imports, whole-RRset deletion, and
DNSSEC mutations are intentionally outside this command set.

## Record input and DNS semantics

```sh
dnscale records create example.com --file examples/record.json --dry-run
dnscale records create example.com --name www --type A --content 192.0.2.10 --ttl 300
dnscale records update example.com RECORD_ID --file replacement.json
dnscale records delete example.com REPLACEMENT_RECORD_ID --yes
```

Files contain one UTF-8 JSON object, up to **1 MiB**. `--file -` reads stdin.
File input cannot be mixed with record field flags. Unknown fields, trailing
JSON documents, and missing name/type/content fail before authentication or
network access. Optional flags are `--ttl`, `--priority`, `--disabled`, and
`--comment`; `--disabled=false`, `--priority 0`, and `--comment ''` are explicit.
JSON preserves `null` for nullable comment and priority fields.

Updates use the API's **PUT** semantics. Name, type, and content are required;
name/type must match the selected record. The CLI does not fetch and merge old
fields. Omitted TTL or `0` means the server default of 3600; omitted disabled
means false. Omitted/null comment preserves it, while an empty string clears
the DNScale-owned comment. TTL and comments apply to the complete RRset. Each
sibling retains its own content and disabled state.

Use the documented TTL range **300–86400 seconds**. The client also accepts
existing API compatibility TTLs below 300 and the zero/default form. MX content
is the target name with priority supplied separately. SRV accepts the full
`priority weight port target` content; the API also supplies defaults for a
target-only value. SOA and platform NS protections are enforced by the server.

`--dry-run` on create/update is entirely offline: no credentials, resolution,
or HTTP requests. It returns the validated request with `submitted: false` and
`validation: "local_only"`. It does not prove DNS validity, ownership, quota,
permissions, or server acceptance. Deletion always requires `--yes` and never
prompts.

## JSON, pagination, and errors

Results use indented JSON; `--json` emits compact JSON. Successful output goes
to stdout and errors to stderr. Help, version, and completion are text.

```json
{
  "data": [],
  "context": {
    "profile": "environment",
    "base_url": "https://api.dnscale.eu/v1",
    "credential_source": "environment"
  },
  "pagination": {
    "offset": 0, "limit": 50, "returned": 0, "total": 0, "has_more": false
  },
  "request_id": "server-request-id"
}
```

`context` is omitted on offline operations. Pagination appears on lists and
includes `next_offset` when more results remain. `--all` prints only after all
pages succeed and rejects inconsistent pagination or repeated IDs. It is not
an atomic snapshot. `request_id`, when supplied by the server, is from the
final request, including when a command resolves a zone name first.

Errors contain `error.code`, `error.message`, and, when available,
`http_status`, `request_id`, and `retry_after_seconds`. Malformed/plain-string
middleware errors retain HTTP metadata and use the SDK's `HTTP_ERROR` fallback.
Credentials are redacted even if an upstream response echoes them.

| Exit | Meaning |
| --- | --- |
| 0 | Success or successful local validation |
| 1 | API, protocol, pagination, or output failure |
| 2 | Invalid arguments, input, or local configuration |
| 3 | Authentication or permission failure (401/403) |
| 4 | Rate limit after eligible read retries |
| 5 | Network failure or timeout |
| 130 | Interrupted |

`--timeout` defaults to **30 seconds for the entire API command**, including
zone resolution, pages, and retry waits. `--retries` defaults to 2 and applies
only to safe reads. Mutations are never automatically retried. A failed or
timed-out mutation may have completed: inspect state before resubmitting.
Regional updates are separate transactions; `PARTIAL_RECORD_UPDATE` requires
checking every affected region. HTTP redirects are rejected. HTTPS is required
except for loopback development URLs.

## API scopes

| Operations | Required scopes/access |
| --- | --- |
| Zone reads | `zones:read` |
| Zone creation | `zones:read`, `zones:write`; customer-wide key |
| Record reads | `zones:read`, `records:read` |
| Record writes | Above, plus `records:write` |
| DNSSEC inspection | `zones:read`, `dnssec:read`; full-zone access |
| Account usage | `usage:read`; customer-wide key |
| Zone usage | `zones:read`, `usage:read`; full-zone access |

The server enforces zone/name restrictions. The CLI never switches credentials
or broadens scope after a denial. DNSSEC inspection does not certify registrar
DS configuration or public propagation and never retrieves private keys.

## Development

```sh
make check           # formatting, race tests, compiled-binary fixtures, vet, modules
make cross-check     # Linux/macOS amd64+arm64, Windows amd64
make build
```

Tests use local fixtures and need no database, real keychain, or live account.

See [the DNS quickstart](QUICKSTART.md), [license](LICENSE), and
[dependency notices](THIRD_PARTY_NOTICES.txt).

Report issues in [dnscaleou/dnscale-cli](https://github.com/dnscaleou/dnscale-cli/issues).
