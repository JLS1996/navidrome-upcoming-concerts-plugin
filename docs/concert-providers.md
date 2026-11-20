# Concert providers (0.4.0)

The target runtime is Navidrome **0.64.2**, using its JSON HTTP, internal Subsonic,
user, library, scheduler and storage host services. The Extism Go PDK is pinned
to **1.1.3**, Go to the 1.25 series and TinyGo to **0.40.1**. The plugin cannot
open a listening port or insert a panel into Navidrome artist pages. Results are
private local HTML and ICS files in plugin storage.

## Ticketmaster setup

1. Register your own free developer key at https://developer.ticketmaster.com/.
2. Install the package, grant exactly one user and the desired libraries, and
   enable the plugin. The library scanner uses `getArtists` for the authorized
   library folders; this may omit artists credited only on individual tracks.
   Names of collaborations are never split heuristically.
3. Open **Concert providers**, enable Ticketmaster and enter the API key in its
   masked field. Keep the default 4,000 requests per UTC day, or choose a lower
   budget. The configured maximum is 4,500.
4. Open the generated dashboard's **Ticketmaster lookup status** section.
   With automatic exact-name matching enabled, a complete search containing one
   exact-name candidate queues concert retrieval. Names are still a heuristic.
   For ambiguous results, supply an explicit attraction ID mapping, for example:

   ```json
   {"your-navidrome-artist-id": "your-ticketmaster-attraction-id"}
   ```

   MusicBrainz-linked identity is preferred where supplied. Name-only matching is
   configurable; ambiguous/truncated searches remain unresolved. Manual mappings
   override automatic identity. Confirming the same automatic ID preserves its
   concerts; choosing a different ID replaces that artist's provider snapshot.

5. The existing refresh switch requeues lookups while respecting the same daily
   request counter and provider cooldown. It does not reset budgets.

No API key is bundled or copied to exported HTML, ICS, or provider state. The key
remains in Navidrome's plugin configuration and is sent only to Ticketmaster's
API. Ticketmaster requires the key in its URL query, so avoid recording outbound
request URLs in your own proxy logs. Production code does not log these URLs.

## Request and refresh behavior

- One provider request per tick, at most one per second. The existing callback
  normally runs every two seconds. MusicBrainz has its own queue.
- Every attempt, including identity searches, retries and pagination, is reserved
  in `ticketmaster.json` **before** HTTP. A failed reservation prevents dispatch.
- The counter survives restarts, library/user scope changes and API-key changes.
  It resets only when the observed UTC date moves forward. Restarts and manual
  refreshes cannot reopen an exhausted day.
- Successful nonempty lookups refresh after 24 hours; empty results after 72
  hours. Unresolved identity searches are retried after seven days or a manual
  refresh. Mappings persist until changed or their artist/scope changes.
- Failures back off exponentially from 30 seconds to 32 minutes and honor
  Retry-After. HTTP 429 and server failures pause the provider queue; 401/403
  pauses requests for an hour. Previous completed concerts remain visible.
- Pagination commits only a complete snapshot. Changed result counts restart
  pagination. More than 1,000 results or five pages reports the provider's deep
  pagination limit and retains the old snapshot; date-range partitioning is not
  implemented yet. Provider counts can still change during pagination, so this
  is not a transactional snapshot of the remote service.
- A local callback lock prevents overlap. Initialization removes an interrupted
  callback's lock. If the host kills a callback without reinitializing the plugin,
  disable and re-enable the plugin to recover the lock.

The local counter governs this installation only. Other installations or tools
using the same key share the remote quota; HTTP 429 still takes precedence.
No paid provider or automatic upgrade is implemented. JamBase is deliberately
absent because its Developer plan bills overages.

## Concert records and presentation

Concerts retain source IDs and links, first/last observed timestamps, local date
and time, timezone and UTC start when supplied, venue, city, region, country,
ticket URLs and provider status. Unknown fields remain unknown. Multi-day concerts remain visible through their
known end date. Date-TBA and
undated entries are shown without assigning a calendar day. Cancelled,
postponed and rescheduled listings are distinguished; disappearance alone does
not set cancellation status.

Shared Ticketmaster event IDs display once with combined library artist credits.
Different event IDs remain separate, including separate same-day performances.
The **Deduplicate concerts across sources** setting is on by default. It merges
unambiguous cross-provider groups using shared authorized library artist IDs and
either a shared canonical event URL, or the same exact day, normalized venue,
city and country with compatible times/ranges. All original provider records stay
in their caches, and merged output keeps each source link. Tracking parameters
are ignored only for comparison; original links remain unchanged.

Different known show times stay separate. If two same-provider shows could both
match another source with no time, the ambiguous group stays separate. Country,
venue or city gaps can prevent a match. Titles alone never establish identity.
Only complete pairwise matching groups merge, with at most one listing per
provider. This intentionally leaves uncertain duplicates visible. Turning the
setting off preserves provider-ID deduplication within each individual source
while showing cross-source listings separately. The setting affects HTML, ICS,
calendar sync and notifications without re-fetching the provider data.

Changing deduplication or enabled sources can change calendar event identities.
Calendar sync can reconcile managed entries after the full refresh completes;
previously imported static ICS files may require manual cleanup.

Calendar exports remain all-day entries. Local times are included in descriptions;
known date ranges use an exclusive ICS end date after the final day. Cancellation
is exported as ICS STATUS:CANCELLED. Google/CalDAV/ntfy remain opt-in; Google
Calendar currently represents status and range details in the description, with
an all-day start-date entry. Live accounts have not been tested in this fork.

## Next expansion

Official artist tour URLs, opt-in JSON-LD/ICS extraction, and additional free
providers are a later phase. No browser, LLM service, public server, paid API or
unrestricted website scraping is required by this implementation.


## SeatGeek setup

You can use SeatGeek and Skiddle with Ticketmaster disabled; no Ticketmaster key
is required. In **Concert providers**, enable SeatGeek, enter your **client ID
(API key)**, and enter a client secret only if your account supplies/requires one.
Both credential fields are masked. Start with the default local budget of 1,000
requests/day, adjusting it to your account limits. This value is a plugin cap,
not a claim about SeatGeek's published allowance.

Complete performer searches with one exact-name candidate can resolve
automatically. Disable **Automatically match a single exact-name artist** for
manual-only name resolution, or override a candidate explicitly:

```json
{"your-navidrome-artist-id": "12345"}
```

The values must be positive numeric SeatGeek performer IDs written as strings.
The adapter queries `/performers` for candidates and `/events` by performer ID;
it validates performer credits and music taxonomy before displaying an event.
Pagination uses SeatGeek's one-based page numbers and reported page size.
Unknown dates stay unknown. When `time_tbd` is true, the documented 03:30
placeholder is discarded, including its UTC equivalent. Venue timezone and UTC
start are retained only when supplied. Public event links and SeatGeek source
attribution appear in the report.

SeatGeek has an independent `seatgeek.json` cache and durable budget with the same
reservation-before-HTTP, refresh and backoff behavior. The safety ceiling is five
pages / 1,000 records per artist. Authentication uses `client_id` and optional
`client_secret` over HTTPS; provider URLs are never logged or stored in caches.
References: https://seatgeek.github.io/

## Skiddle setup

Enable Skiddle, enter the masked API key, and configure artist mappings such as:

```json
{"your-navidrome-artist-id": "45678"}
```

Skiddle's API is for personal, non-commercial use. Its original event links,
source name and unmodified brand logo are retained. The logo is embedded in the
HTML, so viewing the dashboard makes no logo request. Calendar descriptions
retain the Skiddle event and logo URLs.

The adapter searches `/artists/` for candidates, then queries `/events/search/`
with the explicit artist ID (`a`) and `description=1` to retrieve artist credits.
It verifies those credits; it does not guess performers from event titles.
Offset paging uses 100 records per page with a five-page / 500-record safety cap.
The local request budget defaults to 500/day and must fit your account allowance;
Skiddle's actual limits can change. Its budget/cache live in `skiddle.json`,
separate from the other providers.

Requests identify this plugin with its version and project URL. Skiddle calls are
serial and at least three seconds apart, with cached daily results and jittered
refresh/retry times. A 400/401/403/404 response blocks further Skiddle requests;
three failed attempts also stop the provider. Fix the cause and use manual
refresh to requeue it, respecting any remaining cooldown. No extra account or
rotated key is used to bypass limits. A missing artist-credit response is a
lookup failure, not a confirmed empty result.

Doors-open time is labelled separately; it is not substituted for performance
start. Explicit dates, ranges, cancellation flags and venue details are retained.
Live response coverage and differences from the documented API still need testing
with your own configured key.

References: https://www.skiddle.com/api/join.php and
https://github.com/Skiddle/web-api


## Upgrade and recovery notes (0.4)

Old v0.3 candidate caches did not record search completeness. A cached single
candidate is rechecked once under the existing budget before automatic adoption.
No-match caches retain their normal retry schedule. Names are not globally unique,
so review surprising matches or use explicit provider IDs. Existing mappings remain
cached when you disable the heuristic; explicit IDs always win.

Query windows are pinned across paginated callbacks, including midnight. Repeated
pages and changing counts retain the last good snapshot and restart pagination.
Invalid settings preserve saved state. Changing matching preferences does not
reset the authorization scope or daily budgets. Calendar/report filtering happens
after source reconciliation so a newer out-of-window reschedule can supersede an
older listing sharing the same event URL.
