# Upgrade to 0.4.0

Back up plugin storage, then replace the `.ndp` file and rescan/re-enable the
plugin. **Keep existing state and provider JSON files.** You do not need to rescan
all audio or erase daily quota counters.

The key change is automatic matching for a complete search with one exact-name
artist candidate. v0.3 stopped after candidate discovery unless manual IDs were
supplied. The new setting is enabled by default; ambiguous or truncated results
remain unresolved, and manual mappings take precedence. Cached v0.3 single
candidates are rechecked once within existing budgets because their original
search completeness was not recorded. Existing calendar identities are preserved
when replaying an unchanged v0.3 scan.

Other fixes:

- Paginated requests keep the same date window across midnight; repeated or
  inconsistent pages retain good snapshots rather than replace them.
- Ongoing multi-day concerts remain visible through the known end date.
- Source reconciliation happens before date-window filtering, allowing a newer
  reschedule sharing an event URL to supersede an older listing.
- Invalid configuration preserves saved state and reports.
- Matching preferences are separate from user/library authorization scope, so
  authorized provider caches and counters survive setting changes.
- Confirming an automatic ID manually preserves its concerts.
- Persisted provider revisions cause reports to refresh after interrupted
  publication, even when no new request is due.
- Manual refresh resets exhausted retry counts while respecting cooldowns/quotas.

Removed album ownership checks, album artwork fetching, release selectors,
unused release-group parsing, obsolete harnesses and old release documentation.
The direct-credit filter remains useful: artists excluded by it do not count as
pending provider lookups. A completed audio scan and a provider concert refresh
are separate operations.

Names alone are not proof of identity. Review surprising matches, set explicit
provider IDs, or disable **Automatically match a single exact-name artist**.
Previously established mappings remain cached until changed. Use the per-provider
lookup summary to distinguish completed, queued, failed and unresolved lookups.
