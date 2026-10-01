# Changelog

## 1.0.0 — 2026-10-01

- Standalone Go CLI with endpoint-bound keychain profiles and environment auth.
- Zone inventory/creation; record inventory, get, create, update, and selected
  deletion; read-only DNSSEC and usage commands.
- Offline record validation, stable JSON envelopes, opaque IDs, checked
  pagination, command deadlines, and single-attempt writes.
- API-shaped and compiled-binary tests, plus build/package verification for
  Linux and macOS on amd64/arm64 and Windows on amd64.

- Versioned archives with SHA-256 checksums for all five platforms, a public
  `v1.0.0` Go source tag, and Homebrew installation via `dnscaleou/tap/dnscale`.
