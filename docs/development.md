# Development

Target: Navidrome 0.64.2, Extism Go PDK 1.1.3, Go 1.25.x and TinyGo 0.40.1.
The independently written host adapter uses Navidrome's JSON host services.
Extism built-in HTTP is not used.

```sh
go mod download
go test -race -cover ./...
go vet ./...
./scripts/build.sh
go run ./cmd/preview
go run ./cmd/preview -providers -out dist/preview-providers.html
```

The package is `dist/navidrome-upcoming-concerts-plugin.ndp`. Debug metadata is
stripped. No credentials are needed for compilation or deterministic tests.
The source bundle is generated with `python3 scripts/package_source.py` after
reviewing/staging intended source paths; runtime files and credentials must never
be staged. `scripts/check_public.py --tracked` checks the publication file list.

Browser and actual WASM host checks:

```sh
go run ./cmd/preview -providers -out dist/preview-ticketmaster.html
python3 scripts/test_concert_browser.py
NAVIDROME_BIN=/path/to/navidrome python3 scripts/test_concert_integration.py
NAVIDROME_BIN=/path/to/navidrome python3 scripts/test_concert_config.py
```

Integration requires ffmpeg and Navidrome 0.64.2. Browser tests require Python
Playwright and Chromium. Generated audio, credentials and provider responses are
fictional. Integration data stays in ignored `test-runtime/`; previews/builds stay
in ignored `dist/`. No live provider account is used.

`CONCERT_SCAN_DIR` optionally enables a private offline replay test for uploaded
scan files. The test never embeds those files, copies artist names into public
fixtures, or makes requests. Synthetic regression tests cover the underlying bugs.

Additional self-hosted integration hosts can be granted at build time with
`./scripts/build.sh --allow-host calendar.example.org`. Broad wildcards and
credential-bearing URLs are rejected. Compiler/cache overrides are in `scripts/env.sh`.
