# Known limitations

- Coverage depends on enabled sources and is incomplete. Empty results, unresolved
  artist identity and failed lookups are separate outcomes.
- Unique exact-name matching is configurable and can still select a different
  artist sharing that name. Manual provider IDs take precedence.
- Artist enumeration follows the granted Subsonic `getArtists` inventory; artists
  appearing only on track credits may be absent. Collaboration names are not split.
- Conservative deduplication can leave uncertain duplicates. Known distinct show
  times and ambiguous groups remain separate. Calendar identity changes when
  sources/deduplication change can require cleaning old manual ICS imports.
- Calendars use all-day events; supplied local times and doors-open times are
  labelled in descriptions. Unknown timezone/date values are not invented.
- Five-page safety limits retain previous snapshots rather than silently publish
  truncated data. See provider-specific limits in the provider guide.
- Provider counters govern this installation; other clients sharing a key consume
  the remote allowance too. No paid provider or automatic upgrade is implemented.
- Saved HTML is a snapshot and cannot retract copies after permission revocation.
  Keep reports private. Storage access/copying depends on deployment permissions.
  If Navidrome unloads the plugin after all access is removed, existing report
  files remain in plugin storage until you remove them or enable the plugin with
  new grants.
- Official-site extraction, geographic filtering and live-key coverage certification
  are not implemented. Real calendar accounts and real-device sharing need
  deployment checks beyond local fixtures.
