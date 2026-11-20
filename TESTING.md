# Concert plugin 0.4.0 validation

Target: Navidrome 0.64.2 (10114574), Extism Go PDK 1.1.3,
Go 1.25.4 and TinyGo 0.40.1.

## Passed locally

- Native race tests and static analysis; radar coverage above 86%.
- Production WASM package build and source/package privacy checks.
- Offline dashboard search, calendar and provider attribution checks in Chromium
  at 320, 768 and 1280 px, including offline cache-compatibility replay.
- Actual Navidrome/WASM: authorized library enumeration, host HTTP/storage,
  automatic complete-search exact-name resolution through concert retrieval,
  independent quotas, source attribution, paging, cancellation, restart recovery,
  interrupted report publication, and cross-source deduplication toggles without
  extra requests. Malformed configuration preserves the last valid state/report.
- Actual Navidrome configuration UI: credentials masked, mappings/budgets and
  automatic-matching/deduplication switches save and reload correctly.
- Offline replay of v0.3 scan files: all existing calendar UIDs and counts remain
  unchanged; budgets are preserved; only useful incomplete single-candidate
  identities are queued for rechecking. Input data is not part of public fixtures.

Synthetic regressions cover ambiguous/truncated identity searches, manual override
confirmation, partial/repeated/changed pages, midnight date-window pinning,
ongoing multi-day concerts, newer reschedules outside the old display window,
settings versus authorization scope, retry reset, secret redaction and safe writes.

## Reproduce

With Go 1.25 and TinyGo 0.40.1 on PATH:

```sh
go test -race -cover ./...
go vet ./...
./scripts/build.sh
go run ./cmd/preview
go run ./cmd/preview -providers -out dist/preview-ticketmaster.html
python3 scripts/test_concert_browser.py
NAVIDROME_BIN=/path/to/navidrome python3 scripts/test_concert_integration.py
NAVIDROME_BIN=/path/to/navidrome python3 scripts/test_concert_config.py
```

Integration requires ffmpeg and Navidrome 0.64.2; browser tests require Python
Playwright and Chromium. Generated service credentials, audio and provider data
are fictional. Runtime files remain in ignored `test-runtime/`, not release ZIPs.
Browser policy here blocks file URLs, so dashboard checks load generated HTML
without a server. Real-device sharing and deployment file permissions remain
separate checks.

Set `CONCERT_SCAN_DIR` only for an optional private replay of locally supplied
scan files. `CONCERT_REPLAY_REPORT` can save that HTML outside the repository for
private browser verification. Neither option makes provider requests. Do not put
personal exports, matching suggestions or reports in public source fixtures.

## Cleanup and remaining limits

Retired album ownership, artwork fetching, release-group parsing, release selectors,
obsolete release-only harnesses/screenshots and stale documentation. The old JSON
record names remain readable for cache compatibility. Concert authorization,
calendar ownership, escaping and quota regression tests remain in place.

Earlier test builds could complete artist searches without progressing past
unresolved identity. Live concert coverage must be verified after installation
with user-owned keys. No real provider key or calendar account was used here.
Unique names are a configurable heuristic and can still refer to different artists.

Release assets contain reviewed project source and fictional documentation images; private replay inputs and generated private reports are excluded.
