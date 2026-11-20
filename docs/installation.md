# Installation and upgrades

1. Build or obtain `navidrome-upcoming-concerts-plugin.ndp`.
2. Put it in Navidrome's configured plugin folder and rescan plugins.
3. Grant exactly one user and the intended libraries, then enable the plugin.
4. Configure optional providers and integrations in Navidrome's plugin settings.
5. Open the generated `upcoming-concerts.html` in a browser, or copy it from the
   Navidrome host/container. Calendar export is `music-concerts.ics`.

Files are written under
`<DataFolder>/plugins/navidrome-upcoming-concerts-plugin/storage/` with mode 0600.
Navidrome's OS account owns them. Docker users may need a host-side bind mount or
an explicit copy from the container. The plugin has no listening port, arbitrary
API route, public download endpoint or artist-page panel.

For an upgrade, back up existing storage and replace only the `.ndp` package.
Keep `state.json`, provider JSON caches and `integrations.json`: they contain
artist identities, quota counters and calendar/notification acknowledgements.
Version 0.4 reads v0.3 caches; it rechecks cached single candidates whose original
search completeness was not recorded, within the existing provider budget.
There is no need to erase caches or rescan all audio files.

If configuration is invalid, the last valid state and reports are preserved and
Navidrome reports the configuration error. If a callback was interrupted and its
lock remains, disable/re-enable the plugin to reinitialize it.

See [configuration](configuration.md) and [provider setup](concert-providers.md).
