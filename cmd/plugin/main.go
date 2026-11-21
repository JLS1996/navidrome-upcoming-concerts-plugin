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
	s, stateErr := radar.Load(dir)
	c := radar.DefaultConfig()
	finish := func(problem error) error {
		if problem != nil {
			s.Problem = problem.Error()
		}
		return radar.Save(dir, s, c, now)
	}
	hideUntilVerified := func(problem error) error {
		// The prior snapshot stays private in storage and can only be published
		// again after its access scope has been reverified.
		if stateErr == nil && s.Problem == problem.Error() && s.RenderedAt == 0 {
			return nil
		}
		if err := radar.SaveReports(dir, radar.State{Problem: problem.Error()}, c, now); err != nil {
			return err
		}
		if stateErr != nil {
			return stateErr
		}
		s.Problem = problem.Error()
		s.RenderedAt = 0
		return radar.SaveState(dir, s)
	}
	users, err := host.Users()
	if err != nil {
		return hideUntilVerified(fmt.Errorf("unable to verify granted user access"))
	}
	granted := make([]string, 0, len(users))
	for _, u := range users {
		granted = append(granted, u.UserName)
	}
	username, err := radar.ResolveUsername(granted)
	if err != nil {
		return hideUntilVerified(err)
	}
	libs, err := host.Libraries()
	if err != nil {
		return hideUntilVerified(fmt.Errorf("unable to verify granted library access"))
	}
	ids := []int{}
	for _, l := range libs {
		ids = append(ids, int(l.ID))
	}
	rawFolders, err := host.Subsonic("getMusicFolders?u=" + url.QueryEscape(username))
	if err != nil {
		return hideUntilVerified(fmt.Errorf("unable to verify current user library access"))
	}
	ids, err = radar.AccessibleLibraries(rawFolders, ids)
	if err != nil {
		return hideUntilVerified(err)
	}
	if len(ids) == 0 {
		return hideUntilVerified(fmt.Errorf("assign at least one library shared by the selected user and plugin"))
	}
	if stateErr != nil {
		return hideUntilVerified(stateErr)
	}
	var configErr error
	c, configErr = config()
	if configErr != nil {
		if !radar.SameAccessScope(s.Scope, username, ids) {
			return hideUntilVerified(fmt.Errorf("correct the plugin settings to generate a report for the granted libraries"))
		}
		return configErr
	}
	// Keep stable event identities tied to the granted Navidrome account.
	c.Username = username
	ic := integrationConfig(c)
	scope := radar.AccessScope(username, ids)
	if s.LookupConfig == "" && radar.SameAccessScope(s.Scope, username, ids) {
		if at := strings.LastIndex(s.Scope, "|["); at >= 0 {
			s.LookupConfig = s.Scope[:at]
		}
	}
	radar.MigrateScope(&s.Scope, username, ids)
	if s.Scope != scope {
		s = radar.State{Schema: 1, Scope: scope}
	}
	lookupConfig := c.Fingerprint()
	if s.LookupConfig != lookupConfig {
		s.LookupConfig = lookupConfig
		s.RefreshPending = true
		s.NextDiscovery = 0
	}
	// Only publish provider caches after verifying the current access scope.
	providers, providerProblems := loadProviders(dir, &s, c, ids, scope)
	c.DirectReady, c.DirectCredits = s.Direct.Ready, s.Direct.Credits
	previousProviderRefresh := s.ProviderRefreshPending
	var enabledProviders uint8
	for _, provider := range providers {
		enabledProviders |= provider.bit
	}
	s.ProviderRefreshPending &= enabledProviders
	refreshChanged := s.ApplyRefresh(str("manual_refresh", "false") == "true")
	if refreshChanged {
		s.Direct.Refresh = true
		s.ProviderRefreshPending = enabledProviders
	}
	if c.ExcludeUncredited {
		c.DirectReady, c.DirectCredits = s.Direct.Ready, s.Direct.Credits
		if s.Direct.ScanDue(now, s.Direct.Refresh) && now.Unix() >= s.Direct.RetryAt {
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
				if !refreshChanged && strings.HasPrefix(s.Problem, "Checking directly credited music:") && now.Unix()-s.RenderedAt < 30 {
					return radar.SaveState(dir, s)
				}
				return finish(fmt.Errorf("Checking directly credited music: %d albums and %d tracks scanned; integrations are paused", s.Direct.Albums, s.Direct.Tracks))
			}
			c.DirectReady, c.DirectCredits = true, s.Direct.Credits
		}
		if s.Direct.Phase != "" {
			if refreshChanged {
				return radar.SaveState(dir, s)
			}
			return nil
		}
	}
	if s.RefreshPending || now.Unix()-s.DiscoveredAt >= 86400 {
		if now.Unix() < s.NextDiscovery {
			if refreshChanged {
				return radar.SaveState(dir, s)
			}
			return nil
		}
		artists, err := discover(c.Username, ids)
		if err != nil {
			s.NextDiscovery = now.Unix() + 60
			return finish(err)
		}
		s.Reconcile(artists, c, scope, now)
	}
	providerChanged := false
	for _, provider := range providers {
		if provider.state == nil {
			continue
		}
		before, _ := json.Marshal(provider.state)
		next := *provider.state
		next.Reconcile(s.Entries, c, provider.config, scope, s.ProviderRefreshPending&provider.bit != 0)
		after, _ := json.Marshal(next)
		if string(before) != string(after) {
			next.Revision++
			if err := provider.save(dir, next); err != nil {
				providerProblems = append(providerProblems, err.Error())
				continue
			}
			providerChanged = true
		}
		*provider.state = next
		*provider.snapshot = provider.state
		s.ProviderRefreshPending &^= provider.bit
		changed, err := provider.step(provider.state, c, func(endpoint string) (radar.Response, error) {
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{
				"Accept": "application/json", "User-Agent": radar.MusicBrainzUserAgent(),
			}})
		}, func(state radar.TicketmasterState) error { return provider.save(dir, state) }, now)
		if err != nil {
			providerProblems = append(providerProblems, err.Error())
		}
		providerChanged = providerChanged || changed
	}
	previousProblem := s.Problem
	if s.NextDiscovery == 0 {
		s.Problem = strings.Join(providerProblems, " · ")
	}
	view := radar.Version + "/report-2/" + fmt.Sprintf("%d/%d", c.AheadDays, c.PastDays) + fmt.Sprint(c.Skiddle.Enabled, c.SeatGeek.Enabled, c.DeduplicateConcerts, c.Ticketmaster.Enabled, c.Theme, c.ProviderAutoMatch, c.ExcludeUncredited, s.Direct.At) + ic.Fingerprint() + radar.ProviderPublicationVersion(s, c)
	viewChanged := s.ViewConfig != view
	s.ViewConfig = view
	d, loadErr := radar.LoadDeliveries(dir)
	activeIntegrations, configurationProblem := ic.ValidParts()
	deliver := func() bool {
		return loadErr == nil && d.Step(s, c, activeIntegrations, send, now)
	}
	deliveryChanged := false
	if d.PreferDelivery {
		deliveryChanged = deliver()
	}
	changed := false
	if !deliveryChanged {
		changed = s.Step(func(endpoint string) (radar.Response, error) {
			ua := radar.MusicBrainzUserAgent()
			return send(radar.HTTPRequest{Method: "GET", URL: endpoint, Headers: map[string]string{"User-Agent": ua, "Accept": "application/json"}})
		}, c, now)
		if !changed {
			deliveryChanged = deliver()
		}
	}
	if deliveryChanged {
		d.PreferDelivery = false
	} else if changed {
		d.PreferDelivery = true
	}
	previousIntegrationProblem := s.IntegrationProblem
	s.IntegrationProblem = strings.Trim(strings.Join([]string{configurationProblem, d.Status()}, " · "), " ·")
	if loadErr != nil {
		s.IntegrationProblem = loadErr.Error()
	} else if changed || deliveryChanged {
		if err = radar.SaveDeliveries(dir, d); err != nil {
			return err
		}
	}

	// Avoid rewriting the dashboard every idle tick. Render at least hourly for date windows.
	if !providerChanged && !refreshChanged && !changed && !deliveryChanged && !viewChanged && previousProblem == s.Problem && previousProviderRefresh == s.ProviderRefreshPending && previousIntegrationProblem == s.IntegrationProblem && s.DiscoveredAt != now.Unix() && now.Unix()-s.RenderedAt < 3600 {
		return nil
	}
	return finish(nil)
}

// The concert providers share a persistent queue, but load and advance independently.
type concertProvider struct {
	bit      uint8
	config   radar.TicketmasterConfig
	load     func(string) (radar.TicketmasterState, error)
	save     func(string, radar.TicketmasterState) error
	step     func(*radar.TicketmasterState, radar.Config, radar.Fetch, func(radar.TicketmasterState) error, time.Time) (bool, error)
	state    *radar.TicketmasterState
	snapshot **radar.TicketmasterState
}

func loadProviders(dir string, s *radar.State, c radar.Config, ids []int, scope string) ([]concertProvider, []string) {
	providers := []concertProvider{
		{bit: 1, snapshot: &s.Ticketmaster, config: c.Ticketmaster, load: radar.LoadTicketmaster, save: radar.SaveTicketmaster,
			step: func(p *radar.TicketmasterState, c radar.Config, fetch radar.Fetch, save func(radar.TicketmasterState) error, now time.Time) (bool, error) {
				return p.Step(c.Ticketmaster, c, fetch, save, now)
			}},
		{bit: 2, snapshot: &s.SeatGeek, config: c.SeatGeek.QueueConfig(), load: radar.LoadSeatGeek, save: radar.SaveSeatGeek,
			step: func(p *radar.TicketmasterState, c radar.Config, fetch radar.Fetch, save func(radar.TicketmasterState) error, now time.Time) (bool, error) {
				return p.StepSeatGeek(c.SeatGeek, c, fetch, save, now)
			}},
		{bit: 4, snapshot: &s.Skiddle, config: c.Skiddle.QueueConfig(), load: radar.LoadSkiddle, save: radar.SaveSkiddle,
			step: func(p *radar.TicketmasterState, c radar.Config, fetch radar.Fetch, save func(radar.TicketmasterState) error, now time.Time) (bool, error) {
				return p.StepSkiddle(c.Skiddle, c, fetch, save, now)
			}},
	}
	var problems []string
	enabled := providers[:0]
	for _, p := range providers {
		if !p.config.Enabled {
			continue
		}
		state, err := p.load(dir)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			p.state = &state
			radar.MigrateScope(&state.Scope, c.Username, ids)
			if state.Scope == scope {
				*p.snapshot = &state
			}
		}
		enabled = append(enabled, p)
	}
	return enabled, problems
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
