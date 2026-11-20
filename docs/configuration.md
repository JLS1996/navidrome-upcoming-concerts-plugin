# Configuration

Grant exactly one Navidrome user and select the libraries that user can access.
Reports stay in private plugin storage; library write access is unnecessary.

- **Days ahead / back:** the local-date window for displayed concerts.
- **Dashboard theme:** dark, bright or black. Theme changes do not change event IDs.
- **Match artists without IDs:** cautious MusicBrainz name matching. MusicBrainz
  UUIDs from library tags or explicit artist mappings remain preferable.
- **Automatically match a single exact-name artist:** enabled by default for
  external providers. A complete search must return exactly one exact-name
  candidate. Ambiguous/truncated results remain unresolved. Names are a heuristic,
  not proof of identity. Turn this off for manual provider mappings; previously
  resolved mappings remain cached until replaced or the artist identity changes.
- **Deduplicate concerts across sources:** enabled by default. Conservative
  matches retain source links; different or ambiguous performances stay separate.
- **Exclude artists without standalone credits:** only include whole artist
  credits found in scoped albums/tracks. This can leave excluded artists in the
  saved library inventory without querying them or counting them as pending.
- **Manual refresh:** either saved transition queues a refresh, preserving request
  budgets and applicable cooldowns. It also resets exhausted per-artist retry
  counts. A normal Navidrome music scan and a provider concert refresh are separate.

The dashboard shows concert lookup completion, retry/queue counts and unresolved
identities separately for each enabled provider. Unresolved identity is not a
confirmed empty concert result.

See [provider credentials, budgets and mappings](concert-providers.md) and
[calendar/notification settings](integrations.md). Album ownership, release-type
selectors and album cover-art downloads are not concert features and are removed.
Old v0.3 state fields are ignored where no longer needed; library and provider
caches and daily request counts are retained during upgrades.
