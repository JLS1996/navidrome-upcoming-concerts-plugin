# Architecture and implementation notes

The plugin connects Navidrome's authorized artist inventory to independent concert
providers and generates private static reports. It uses Navidrome's JSON host
services for HTTP, storage, scheduling, users, libraries and internal Subsonic calls.

## Processing

1. Verify the granted user and current library permissions.
2. Enumerate artists from accessible libraries; optionally verify whole artist
   credits against scoped albums/tracks without splitting collaboration names.
3. Reconcile provider mappings and enqueue eligible identity/event lookups.
4. Reserve each request in persistent storage before dispatching HTTP.
5. Normalize complete provider snapshots, retaining previous results on errors.
6. Reconcile shared identities and conservative cross-source matches.
7. Publish local HTML/ICS and process explicitly enabled external integrations.

## Provider and concert state

Concerts retain source IDs/links, supplied local dates/times and timezone, venue
location, status and first/last observation timestamps. Unknown data remains
unknown. Performer/artist/attraction IDs are provider-specific; manual IDs take
precedence over name heuristics.

Each optional provider has separate persistent mappings, pagination, request
counters and cooldowns. User/library authorization scope is distinct from matching
and presentation settings. Daily budgets survive callback restarts, key changes
and configuration updates. Completed provider revisions invalidate reports even
if a previous callback stopped before publishing HTML/ICS.

Legacy cache record names remain readable for v0.3 upgrades. Artist-name adoption
requires a complete search with one exact-name candidate when the configurable
heuristic is enabled. Older single-candidate caches are rechecked within the same
budget because they did not record search completeness.

## Output and deduplication

All outputs use the same filtered concert view. Shared provider event IDs display
once. Cross-source deduplication uses authorized artist identity with canonical
event URLs or compatible date, venue and location details. Ambiguous groups and
different performances remain separate; merged rows retain source links.

HTML is self-contained and escaped. ICS uses all-day events and supplied ranges,
with uncertain dates omitted and source details retained in descriptions. External
calendar/notification delivery is optional, with persisted acknowledgements and
ownership/precondition checks.

## Validation and future work

Native race/static checks, headless browser checks and actual Navidrome/WASM
integration exercise source identity, quotas, paging, rendering, upgrades and
restart recovery. Service tests use fictional data. Optional offline cache replay
supports local compatibility verification without publishing input data.

Future work includes opt-in official-site JSON-LD/ICS adapters, broader live
coverage checks, geographic filters and date partitioning for large provider
result sets. These features are not implied by the current release.
