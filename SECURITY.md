# Privacy and security

The plugin has no HTTP listener, public download route, calendar subscription
endpoint or unauthenticated Navidrome panel. It writes private local HTML and ICS
snapshots. Do not expose those files through a public web directory or proxy.

Every callback verifies the granted user and current library access before using
cached artists. Authorization scope is separate from matching/display preferences:
changing a setting retains authorized caches, while user/library changes remove
unrelated results. Previously copied snapshots cannot be retracted automatically.

Provider calls use explicit host grants and private administrator-supplied keys.
Only individual artist/event queries are sent, not the whole library inventory.
Keys stay in Navidrome configuration, with masked controls; they are not written
into provider state, HTML, ICS, error messages or source bundles. Provider APIs
require credentials in query strings, so avoid logging their outbound URLs in
proxies. Redirects are not followed with credentials. No paid API or automatic
account upgrade is implemented.

State, reports and integration acknowledgements are private working data. Daily
request reservations are persisted before HTTP, remain across settings changes
and restarts, and honor provider cooldowns. Corrupt state fails closed. Invalid
configuration leaves the last valid state/reports untouched. Keep backups when
upgrading; do not reset quota caches to bypass provider limits.

Notifications and calendar sync are off by default and send data only to configured
HTTPS destinations. A public ntfy topic is not private merely because publishing
uses a token. Use a dedicated calendar: ownership markers and ETag/precondition
checks protect unrelated events. Self-hosted hosts need explicit permissions;
never use a universal wildcard to bypass a network error.

Skiddle attribution uses its original logo embedded in the dashboard; opening the
file does not request images from a third party. Event links require internet when
clicked. Album artwork fetching and ownership scans have been removed.

Normal tests use fictional artists and credentials. Optional private scan replay
is offline; personal exports, runtime directories and keys must never be staged,
published or included in release archives. The source packaging/public-file checks
provide an additional check, not a substitute for reviewing the file list.

For sensitive reports, use private vulnerability reporting where available. Do not
post personal library exports, credentials or tokens in a public issue. Local tests
do not replace an independent security review.
