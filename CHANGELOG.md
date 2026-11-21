# Changelog

## 0.5.0 — 2026-11-21

- Replace the inherited release dashboard with an offline concert agenda,
  separate upcoming/recent/uncertain-date sections and a compact month calendar.
- Search by artist, show, venue or city and filter by timing, listing status or
  source. A matched concert remains findable under every contributing provider.
- Keep Today and Ongoing shows in the Upcoming filter. Keep cancelled and
  rescheduled listings in their correct timing groups.
- Jump to the matching month when a calendar search finds dates elsewhere.
  Preserve keyboard focus when moving between calendar days and months.
- Preserve multi-day MusicBrainz dates in the agenda and calendar exports.
- Remove the empty legacy Concerts settings tab. Move lookup diagnostics into
  a disclosure below the report.

## 0.4.1 — 2026-11-21

- Verify user and library access even when settings are invalid, and hide local
  reports when their cached scope cannot be verified.
- Keep MusicBrainz scans and local exports working when the integration cache
  needs recovery.
- Require an ownership check and ETag before removing an unconfirmed calendar
  event. Defer removals while MusicBrainz lookups are failing.

- Preserve multi-day ranges and cancellation status in Google Calendar sync;
  repair existing managed exports once without changing event identities.
- Keep each concert's details and attribution separate when source URLs match or
  are unavailable.
- Show multi-day concerts on every affected calendar day, including ranges that
  cross months; retain fully undated shows in the uncertain-date list.
- Add native and browser regressions for calendar payloads, cache repair, shared
  URLs, multi-day display and undated concerts.

- Keep authorized provider concerts in the dashboard and ICS during artist-credit
  rescans. Carry manual refreshes through those scans, and keep healthy sources
  available if another provider cache needs recovery.
- Hide reports when access cannot be verified. Reuse the cached scan after the
  same user and library grants are confirmed again.
- Show concert details and provider attribution in the selected-day view.
  When a listing URL is unavailable, display the title as text and omit the
  URL property from calendar exports.

## 0.4.0 — 2026-11-20

First public release of the concerts plugin.

- Discover artist concerts through optional Ticketmaster Discovery, SeatGeek and
  Skiddle adapters, with supplementary MusicBrainz event data.
- Browse a private offline list/calendar dashboard, use three themes, export ICS,
  and opt into ntfy, CalDAV or Google Calendar integrations.
- Keep independent durable provider budgets, mappings, resumable pagination and
  completed snapshots through failed refreshes.
- Deduplicate clear cross-source matches while retaining each provider's event
  links and attribution; preserve distinct or ambiguous performances.
- Resolve a single exact-name candidate after a complete search when the
  configurable heuristic is enabled; support explicit provider-ID mappings.
- Read v0.3 test-build caches and preserve counters and existing calendar identities
  during unchanged-cache compatibility replay.
- Fix midnight pagination windows, repeated pages, ongoing date ranges, reschedules
  outside the old display window, invalid-setting state loss and interrupted
  report publication.
- Separate matching preferences from authorization scope and retain cached concerts
  when confirming an automatic match manually.
- Retire album artwork, ownership checks, release selectors and unused release
  lookup/harness code.

See [release notes](docs/releases/v0.4.0.md) for installation, validation and limits.

## Lineage

Derived from version 0.8.1 of
[navidrome-upcoming-releases-plugin](https://github.com/JLS1996/navidrome-upcoming-releases-plugin).
Versions 0.1–0.3 of this concerts adaptation were local development builds.
The parent project's history is documented in its own repository.
