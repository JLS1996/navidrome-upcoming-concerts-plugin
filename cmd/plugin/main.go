//go:build wasip1

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	host "github.com/JLS1996/navidrome-upcoming-concerts-plugin/internal/navhost"
	"github.com/JLS1996/navidrome-upcoming-concerts-plugin/internal/radar"
	"github.com/extism/go-pdk"
)

type plugin struct{}

func exported(err error) int32 {
	if err != nil {
		pdk.SetError(err)
		return -1
	}
	return 0
}

//go:wasmexport nd_on_init
func initialize() int32 { return exported((&plugin{}).OnInit()) }

//go:wasmexport nd_scheduler_callback
func scheduled() int32 { return exported((&plugin{}).tick()) }
func main()            {}
func (p *plugin) OnInit() error {
	// Navidrome initializes this plugin before scheduling its callbacks.
	if dir, err := host.StorageRoot(); err == nil {
		if err = os.Remove(filepath.Join(dir, "tick.lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := host.Schedule("@every 2s", "tick", "concert-radar-tick"); err != nil {
		return err
	}
	return p.tick()
}
func str(key, def string) string {
	v, ok := host.ConfigValue(key)
	if !ok || v == "" {
		return def
	}
	return v
}
func config() (radar.Config, error) {
	c := radar.DefaultConfig()
	c.Theme = str("dashboard_theme", "dark")
	c.ExcludeUncredited = str("exclude_uncredited", "false") == "true"
	c.CalendarNamespace = str("calendar_namespace", "navidrome-upcoming-concerts-plugin")
	var err error
	c.AheadDays, err = strconv.Atoi(str("ahead_days", "90"))
	if err != nil {
		return c, fmt.Errorf("ahead_days must be an integer")
	}
	c.PastDays, err = strconv.Atoi(str("past_days", "0"))
	if err != nil {
		return c, fmt.Errorf("past_days must be an integer")
	}
	c.AutoMatch = str("auto_match", "true") == "true"
	if err = json.Unmarshal([]byte(str("artist_overrides", "{}")), &c.Overrides); err != nil || c.Overrides == nil {
		return c, fmt.Errorf("artist_overrides must be a JSON object of Navidrome ID to MusicBrainz ID")
	}
	var excluded []string
	if err = json.Unmarshal([]byte(str("excluded_artists", "[]")), &excluded); err != nil || excluded == nil {
		return c, fmt.Errorf("excluded_artists must be a JSON array of Navidrome IDs")
	}
	for _, id := range excluded {
		c.Exclusions[id] = true
	}
	c.Ticketmaster.Enabled = str("ticketmaster_enabled", "false") == "true"
	c.Ticketmaster.APIKey = str("ticketmaster_api_key", "")
	c.Ticketmaster.DailyBudget, err = strconv.Atoi(str("ticketmaster_daily_budget", "4000"))
	if err != nil {
		return c, fmt.Errorf("invalid Ticketmaster daily budget")
	}
	if err = json.Unmarshal([]byte(str("ticketmaster_artist_overrides", "{}")), &c.Ticketmaster.Overrides); err != nil || c.Ticketmaster.Overrides == nil {
		return c, fmt.Errorf("Ticketmaster overrides must map Navidrome artist IDs to attraction IDs")
	}
	if err = c.Ticketmaster.Validate(); err != nil {
		return c, err
	}
	c.DeduplicateConcerts = str("deduplicate_concerts", "true") == "true"
	c.ProviderAutoMatch = str("provider_auto_match", "true") == "true"
	c.SeatGeek.Enabled = str("seatgeek_enabled", "false") == "true"
	c.SeatGeek.ClientID = str("seatgeek_client_id", "")
	c.SeatGeek.ClientSecret = str("seatgeek_client_secret", "")
	c.SeatGeek.DailyBudget, err = strconv.Atoi(str("seatgeek_daily_budget", "1000"))
	if err != nil {
		return c, fmt.Errorf("invalid SeatGeek local daily budget")
	}
	if err = json.Unmarshal([]byte(str("seatgeek_artist_overrides", "{}")), &c.SeatGeek.Overrides); err != nil || c.SeatGeek.Overrides == nil {
		return c, fmt.Errorf("SeatGeek mappings must map Navidrome artist IDs to numeric performer IDs as strings")
	}
	if err = c.SeatGeek.Validate(); err != nil {
		return c, err
	}
	c.Skiddle.Enabled = str("skiddle_enabled", "false") == "true"
	c.Skiddle.APIKey = str("skiddle_api_key", "")
	c.Skiddle.DailyBudget, err = strconv.Atoi(str("skiddle_daily_budget", "500"))
	if err != nil {
		return c, fmt.Errorf("invalid Skiddle local budget")
	}
	if err = json.Unmarshal([]byte(str("skiddle_artist_overrides", "{}")), &c.Skiddle.Overrides); err != nil || c.Skiddle.Overrides == nil {
		return c, fmt.Errorf("Skiddle mappings must map Navidrome artist IDs to numeric artist IDs as strings")
	}
	if err = c.Skiddle.Validate(); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func integrationConfig(c radar.Config) radar.IntegrationConfig {
	return radar.IntegrationConfig{
		Namespace: c.EventNamespace(), NtfyEnabled: str("ntfy_enabled", "false") == "true", NotifyNew: str("ntfy_notify_new", "true") == "true", NotifyUpdates: str("ntfy_notify_updates", "true") == "true", NotifyReleaseDay: str("ntfy_notify_concert_day", "false") == "true", NtfyURL: str("ntfy_server", "https://ntfy.sh"), NtfyTopic: str("ntfy_topic", ""), NtfyToken: str("ntfy_access_token", ""), NotifyInitial: str("ntfy_notify_initial", "false") == "true",
		CalendarProvider: str("calendar_provider", "none"), CalDAVURL: str("caldav_calendar_url", ""), CalDAVUser: str("caldav_username", ""), CalDAVPassword: str("caldav_app_password", ""),
		GoogleCalendarID: str("google_calendar_id", ""), GoogleClientID: str("google_client_id", ""), GoogleClientSecret: str("google_client_secret", ""), GoogleRefreshToken: str("google_refresh_token", "")}
}
func send(req radar.HTTPRequest) (radar.Response, error) {
	response, err := host.Do(host.Request{Method: req.Method, URL: req.URL, Headers: req.Headers, Body: req.Body, TimeoutMs: 6000, NoFollowRedirects: true})
	if err != nil {
		return radar.Response{}, err
	}
	if response == nil {
		return radar.Response{}, fmt.Errorf("empty HTTP host response")
	}
	return radar.Response{Status: int(response.StatusCode), Headers: response.Headers, Body: response.Body}, nil
}
func (p *plugin) tick() error {
	now := time.Now().UTC()
	dir, err := host.StorageRoot()
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "tick.lock")
	lock, lockErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(lockErr) {
		return nil
	}
	if lockErr != nil {
		return lockErr
	}
	lock.Close()
	defer os.Remove(lockPath)
	s, err := radar.Load(dir)
	if err != nil {
		return err
	}
	c, configErr := config()
	ic := integrationConfig(c)
	finish := func(problem error) error {
		if problem != nil {
			s.Problem = problem.Error()
		}
		return radar.Save(dir, s, c, now)
	}
	deny := func(problem error) error { s = radar.State{Schema: 1}; return finish(problem) }
	if configErr != nil {
		// A typo in settings must not erase the last valid library snapshot.
		return configErr
	}
	users, err := host.Users()
	if err != nil {
		return deny(fmt.Errorf("unable to verify granted user access"))
	}
	granted := make([]string, 0, len(users))
	for _, u := range users {
		granted = append(granted, u.UserName)
	}
	c.Username, err = radar.ResolveUsername(granted)
	if err != nil {
		s = radar.State{Schema: 1}
		return finish(err)
	}
	// Keep stable event identities tied to the granted Navidrome account.
	ic = integrationConfig(c)
	libs, err := host.Libraries()
	if err != nil {
		return deny(fmt.Errorf("unable to verify granted library access"))
	}
	ids := []int{}
	for _, l := range libs {
		ids = append(ids, int(l.ID))
	}
	rawFolders, err := host.Subsonic("getMusicFolders?u=" + url.QueryEscape(c.Username))
	if err != nil {
		return deny(fmt.Errorf("unable to verify current user library access"))
	}
	ids, err = radar.AccessibleLibraries(rawFolders, ids)
	if err != nil {
		return deny(err)
	}
	scope := radar.AccessScope(c.Username, ids)
	if s.LookupConfig == "" && radar.SameAccessScope(s.Scope, c.Username, ids) {
		if at := strings.LastIndex(s.Scope, "|["); at >= 0 {
			s.LookupConfig = s.Scope[:at]
		}
	}
	radar.MigrateScope(&s.Scope, c.Username, ids)
	if s.Scope != scope {
		s = radar.State{Schema: 1, Scope: scope}
	}
	if len(ids) == 0 {
		s = radar.State{Schema: 1}
		return finish(fmt.Errorf("assign at least one library shared by the selected user and plugin"))
	}
	lookupConfig := c.Fingerprint()
	if s.LookupConfig != lookupConfig {
		s.LookupConfig = lookupConfig
		s.RefreshPending = true
		s.NextDiscovery = 0
	}
	c.DirectReady, c.DirectCredits = s.Direct.Ready, s.Direct.Credits
	refreshChanged := s.ApplyRefresh(str("manual_refresh", "false") == "true")
	if refreshChanged {
		s.Direct.At = 0
	}
	if c.ExcludeUncredited {
		c.DirectReady, c.DirectCredits = s.Direct.Ready, s.Direct.Credits
		if s.Direct.ScanDue(now, refreshChanged) && now.Unix() >= s.Direct.RetryAt {
			q := url.Values{"u": {c.Username}}
			for _, id := range ids {
				q.Add("musicFolderId", strconv.Itoa(id))
			}
			endpoint := "getAlbumList2"
			if s.Direct.Phase == "albums" {
				q.Set("type", "alphabeticalByName")
				q.Set("size", "500")
				q.Set("offset", strconv.Itoa(s.Direct.Offset))
			} else {
				endpoint = "search3"
				q.Set("query", "")
				q.Set("artistCount", "0")
				q.Set("albumCount", "0")
				q.Set("songCount", "500")
				q.Set("songOffset", strconv.Itoa(s.Direct.Offset))
			}
			raw, scanErr := host.Subsonic(endpoint + "?" + q.Encode())
			complete := false
			if scanErr == nil {
				complete, scanErr = s.Direct.Page(raw, now)
			}
			if scanErr != nil {
				s.Direct.Restart(now)
				return finish(fmt.Errorf("direct-credit scan failed: %v; retrying in one minute", scanErr))
			}
			if !complete {
				return finish(fmt.Errorf("Checking directly credited music: %d albums and %d tracks scanned; integrations are paused", s.Direct.Albums, s.Direct.Tracks))
			}
			c.DirectReady, c.DirectCredits = true, s.Direct.Credits
		}
		if s.Direct.Phase != "" {
			return finish(fmt.Errorf("Direct-credit scan is waiting to retry; integrations are paused"))
		}
	}
	if (s.RefreshPending || now.Unix()-s.DiscoveredAt >= 86400) && now.Unix() >= s.NextDiscovery {
		artists, err := discover(c.Username, ids)
		if err != nil {
			s.NextDiscovery = now.Unix() + 60
			return finish(err)
		}
		s.Reconcile(artists, c, scope, now)
	}
	providerChanged := false
	if c.Ticketmaster.Enabled {
		provider, loadErr := radar.LoadTicketmaster(dir)
		if loadErr != nil {
			return finish(loadErr)
		}
		providerBefore, _ := json.Marshal(provider)
		radar.MigrateScope(&provider.Scope, c.Username, ids)
		provider.Reconcile(s.Entries, c, c.Ticketmaster, scope, refreshChanged)
		providerChanged, err = provider.Step(c.Ticketmaster, c, func(endpoint string) (radar.Response, error) {
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{"Accept": "application/json"}})
		}, func(state radar.TicketmasterState) error { return radar.SaveTicketmaster(dir, state) }, now)
		if err != nil {
			return finish(err)
		}
		providerAfter, _ := json.Marshal(provider)
		if !providerChanged && string(providerBefore) != string(providerAfter) {
			if err = radar.SaveTicketmaster(dir, provider); err != nil {
				return finish(err)
			}
		}
		s.Ticketmaster = &provider
	}
	if c.SeatGeek.Enabled {
		provider, loadErr := radar.LoadSeatGeek(dir)
		if loadErr != nil {
			return finish(loadErr)
		}
		before, _ := json.Marshal(provider)
		radar.MigrateScope(&provider.Scope, c.Username, ids)
		provider.Reconcile(s.Entries, c, c.SeatGeek.QueueConfig(), scope, refreshChanged)
		sgChanged, stepErr := provider.StepSeatGeek(c.SeatGeek, c, func(endpoint string) (radar.Response, error) {
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{"Accept": "application/json"}})
		}, func(state radar.SeatGeekState) error { return radar.SaveSeatGeek(dir, state) }, now)
		if stepErr != nil {
			return finish(stepErr)
		}
		after, _ := json.Marshal(provider)
		if !sgChanged && string(before) != string(after) {
			if err = radar.SaveSeatGeek(dir, provider); err != nil {
				return finish(err)
			}
		}
		s.SeatGeek = &provider
		providerChanged = providerChanged || sgChanged
	}
	if c.Skiddle.Enabled {
		provider, loadErr := radar.LoadSkiddle(dir)
		if loadErr != nil {
			return finish(loadErr)
		}
		before, _ := json.Marshal(provider)
		radar.MigrateScope(&provider.Scope, c.Username, ids)
		provider.Reconcile(s.Entries, c, c.Skiddle.QueueConfig(), scope, refreshChanged)
		skChanged, stepErr := provider.StepSkiddle(c.Skiddle, c, func(endpoint string) (radar.Response, error) {
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{"Accept": "application/json", "User-Agent": radar.MusicBrainzUserAgent()}})
		}, func(state radar.SkiddleState) error { return radar.SaveSkiddle(dir, state) }, now)
		if stepErr != nil {
			return finish(stepErr)
		}
		after, _ := json.Marshal(provider)
		if !skChanged && string(before) != string(after) {
			if err = radar.SaveSkiddle(dir, provider); err != nil {
				return finish(err)
			}
		}
		s.Skiddle = &provider
		providerChanged = providerChanged || skChanged
	}
	previousProblem := s.Problem
	if s.NextDiscovery == 0 {
		s.Problem = ""
	}
	view := radar.Version + "/" + fmt.Sprintf("%d/%d", c.AheadDays, c.PastDays) + fmt.Sprint(c.Skiddle.Enabled, c.SeatGeek.Enabled, c.DeduplicateConcerts, c.Ticketmaster.Enabled, c.Theme, c.ProviderAutoMatch, c.ExcludeUncredited, s.Direct.At) + ic.Fingerprint() + radar.ProviderPublicationVersion(s, c)
	viewChanged := s.ViewConfig != view
	s.ViewConfig = view
	d, loadErr := radar.LoadDeliveries(dir)
	if loadErr != nil {
		return finish(loadErr)
	}
	activeIntegrations, configurationProblem := ic.ValidParts()
	deliveryChanged := false
	if d.PreferDelivery {
		deliveryChanged = d.Step(s, c, activeIntegrations, send, now)
	}
	changed := false
	if !deliveryChanged {
		changed = s.Step(func(endpoint string) (radar.Response, error) {
			ua := radar.MusicBrainzUserAgent()
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{"User-Agent": ua, "Accept": "application/json"}})
		}, c, now)
		if !changed {
			deliveryChanged = d.Step(s, c, activeIntegrations, send, now)
		}
	}
	if deliveryChanged {
		d.PreferDelivery = false
	} else if changed {
		d.PreferDelivery = true
	}
	previousIntegrationProblem := s.IntegrationProblem
	s.IntegrationProblem = strings.Trim(strings.Join([]string{configurationProblem, d.Status()}, " · "), " ·")
	if changed || deliveryChanged {
		if err = radar.SaveDeliveries(dir, d); err != nil {
			return err
		}
	}

	// Avoid rewriting the dashboard every idle tick. Render at least hourly for date windows.
	if !providerChanged && !refreshChanged && !changed && !deliveryChanged && !viewChanged && previousProblem == "" && previousIntegrationProblem == s.IntegrationProblem && s.DiscoveredAt != now.Unix() && now.Unix()-s.RenderedAt < 3600 {
		return nil
	}
	return finish(nil)
}
func discover(username string, ids []int) ([]radar.Artist, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no verified libraries were selected")
	}
	q := url.Values{"u": {username}}
	for _, id := range ids {
		q.Add("musicFolderId", strconv.Itoa(id))
	}
	raw, err := host.Subsonic("getArtists?" + q.Encode())
	if err != nil {
		return nil, err
	}
	artists, err := radar.ParseArtists(raw)
	if err != nil {
		return nil, err
	}
	result := []radar.Artist{}
	for _, a := range artists {
		if !strings.EqualFold(strings.TrimSpace(a.Name), "Various Artists") {
			result = append(result, a)
		}
	}
	return result, nil
}
