# Calendars and notifications

All integrations are optional and disabled by default. Reports stay in local
plugin storage. Only destinations you configure receive data.

## ICS export

The dashboard includes calendar text and an ICS download for the full filtered
report. Temporary browser search filters do not alter the exported calendar.
Unknown/partial dates are excluded. Events are all-day, with source links and any
known local time/doors-open time in descriptions. Date ranges use an exclusive end
date after the last day. Cancelled listings have ICS `STATUS:CANCELLED`.

## Calendar sync

Use a dedicated calendar. CalDAV requires the full writable HTTPS collection URL,
a username and an app password. Google Calendar requires a calendar ID and OAuth
client/refresh credentials; `scripts/google_authorize.py` helps obtain them.
Credentials are masked in configuration, not written to reports. Self-hosted
servers require their hostname in the package's HTTP permission grants.

Only events managed by this plugin are updated/deleted, with ownership and
precondition checks. Incomplete or failing provider refreshes prevent destructive
calendar reconciliation. Keep `integrations.json` to preserve acknowledgements.
If that file is damaged, local scans and reports continue while remote delivery
pauses. The dashboard reports the problem so you can restore the delivery state
from a backup.
Google and CalDAV preserve multi-day ranges and cancellation status. Google
exports created by older versions are updated once using the existing event IDs;
notification acknowledgements are preserved. Live accounts remain unverified in
this fork; local protocol fixtures cover CRUD and conflict recovery.

## ntfy

Configure server, topic and optional access token. Choose discovery, update and
concert-day notices; importing the initial backlog is separately configurable.
Reminder deduplication uses UTC. Source attribution and links are retained.

Changing provider or deduplication settings changes the shared output used by
HTML, ICS, sync and notifications. Existing manual ICS imports may need cleanup
when event identities change. No real account is contacted by normal tests.
