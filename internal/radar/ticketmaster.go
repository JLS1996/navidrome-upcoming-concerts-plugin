package radar

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ArtistCandidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var ticketmasterBase = "https://app.ticketmaster.com/discovery/v2/" // Fixture override is build-tagged, never configured.

type ProviderArtist struct {
	CandidatesComplete bool              `json:"candidatesComplete,omitempty"`
	QueryDay           string            `json:"queryDay,omitempty"`
	ExpectedTotal      int               `json:"expectedTotal"`
	Received           int               `json:"received"`
	Candidates         []ArtistCandidate `json:"candidates,omitempty"`
	Artist             Artist            `json:"artist"`
	Mapping            ArtistMapping     `json:"mapping"`
	Lookup             LookupState       `json:"lookup"`
	Concerts           []Concert         `json:"concerts"`
	Pending            []Concert         `json:"pending,omitempty"`
	Window             string            `json:"window"`
}
type TicketmasterState struct {
	Revision       uint64           `json:"revision,omitempty"`
	MatchingPolicy int              `json:"matchingPolicy,omitempty"`
	Blocked        bool             `json:"blocked,omitempty"`
	Schema         int              `json:"schema"`
	Scope          string           `json:"scope"`
	Artists        []ProviderArtist `json:"artists"`
	BudgetDay      string           `json:"budgetDay"`
	Used           int              `json:"requestsReserved"`
	NextHTTP       int64            `json:"nextHttp"`
	PauseUntil     int64            `json:"pauseUntil"`
	Cursor         int              `json:"cursor"`
}

func (s *TicketmasterState) Reconcile(entries []Entry, c Config, tc TicketmasterConfig, scope string, refresh bool) {
	old := map[string]ProviderArtist{}
	if s.Scope == scope {
		for _, a := range s.Artists {
			old[a.Artist.ID] = a
		}
	}
	next := []ProviderArtist{}
	window := fmt.Sprintf("%d/%d", c.PastDays, c.AheadDays)
	for _, e := range entries {
		if !IncludedArtist(e.Artist, c) {
			continue
		}
		a, ok := old[e.Artist.ID]
		override := tc.Overrides[e.Artist.ID]
		if ok && a.Artist.Name == e.Artist.Name && override != "" && override == a.Mapping.ProviderID {
			// Confirming an existing automatic match must not discard its concerts.
			a.Mapping.ManualOverride = override
			a.Mapping.MatchMethod = "manual"
		} else if !ok || a.Artist.Name != e.Artist.Name || a.Mapping.ManualOverride != override {
			a = ProviderArtist{Mapping: ArtistMapping{NavidromeID: e.Artist.ID, MusicBrainzID: e.Artist.MBID, ProviderID: override, ManualOverride: override, MatchMethod: "unresolved"}}
			if override != "" {
				a.Mapping.MatchMethod = "manual"
			}
		}
		a.Artist = e.Artist
		a.Mapping.MusicBrainzID = e.ResolvedMBID
		if a.Mapping.MusicBrainzID == "" {
			a.Mapping.MusicBrainzID = e.Artist.MBID
		}
		if refresh || a.Window != window {
			a.Lookup.NextRefresh = 0
			a.Lookup.Page = 0
			a.Pending = nil
			if refresh {
				a.Lookup.Failures = 0
				a.Lookup.LastError = ""
			}
		}
		// v0.3 caches did not record whether all candidates were returned.
		// Recheck only useful single candidates once, within the existing budget.
		if c.ProviderAutoMatch && s.MatchingPolicy != 1 && a.Lookup.Outcome == "unresolved" && len(a.Candidates) == 1 {
			a.Lookup.NextRefresh = 0
		}
		if c.ProviderAutoMatch {
			a.adoptUniqueCandidate()
		}
		a.Window = window
		next = append(next, a)
	}
	// Budget is deliberately preserved across scope, key, and library changes.
	if refresh {
		s.Blocked = false
	}
	s.Schema = 1
	if c.ProviderAutoMatch {
		s.MatchingPolicy = 1
	} else {
		s.MatchingPolicy = 2
	}
	s.Scope = scope
	s.Artists = next
}
func (s TicketmasterState) Validate() error {
	if s.Schema != 1 || s.Used < 0 || s.NextHTTP < 0 || s.PauseUntil < 0 {
		return fmt.Errorf("invalid Ticketmaster state")
	}
	if s.BudgetDay != "" {
		if _, err := time.Parse("2006-01-02", s.BudgetDay); err != nil {
			return fmt.Errorf("invalid Ticketmaster budget date")
		}
	}
	seen := map[string]bool{}
	for _, a := range s.Artists {
		if a.QueryDay != "" {
			if _, err := time.Parse("2006-01-02", a.QueryDay); err != nil {
				return fmt.Errorf("invalid provider pagination date")
			}
		}
		if a.Artist.ID == "" || seen[a.Artist.ID] || a.Lookup.Page < 0 || a.Lookup.Page >= 5 || a.Lookup.Failures < 0 || a.Lookup.Failures > 7 || a.Lookup.NextRefresh < 0 || a.Received < 0 || a.Received > 1000 || a.ExpectedTotal < 0 || a.ExpectedTotal > 1000 || len(a.Pending) > 1000 || len(a.Concerts) > 1000 || len(a.Candidates) > 20 {
			return fmt.Errorf("invalid Ticketmaster artist state")
		}
		if a.Mapping.ProviderID != "" && !validProviderID(a.Mapping.ProviderID) {
			return fmt.Errorf("invalid cached Ticketmaster attraction")
		}
		seen[a.Artist.ID] = true
	}
	return nil
}

// Step performs at most one request. persist MUST durably save this state before
// dispatch; reservation failure prevents the HTTP call. Errors never include URLs.
func (s *TicketmasterState) Step(tc TicketmasterConfig, c Config, fetch Fetch, persist func(TicketmasterState) error, now time.Time) (bool, error) {
	return s.step(tc, c, fetch, persist, now, providerAdapter{name: "Ticketmaster", request: func(a *ProviderArtist, c Config, now time.Time) string {
		q := url.Values{"apikey": {tc.APIKey}, "size": {"200"}}
		path := "events.json"
		if a.Mapping.ProviderID == "" {
			path = "attractions.json"
			q.Set("keyword", a.Artist.Name)
			q.Set("classificationName", "music")
		} else {
			// Pad UTC boundaries; final visibility uses the supplied local date.
			today := providerQueryDay(a, now)
			q.Set("attractionId", a.Mapping.ProviderID)
			q.Set("classificationName", "music")
			q.Set("page", strconv.Itoa(a.Lookup.Page))
			q.Set("sort", "date,asc")
			q.Set("startDateTime", today.AddDate(0, 0, -c.PastDays-1).Format("2006-01-02T15:04:05Z"))
			q.Set("endDateTime", today.AddDate(0, 0, c.AheadDays+2).Add(-time.Second).Format("2006-01-02T15:04:05Z"))
		}
		return ticketmasterBase + path + "?" + q.Encode()
	}, resolve: (*ProviderArtist).resolve, events: (*ProviderArtist).events})
}

type providerAdapter struct {
	interval int64
	name     string
	request  func(*ProviderArtist, Config, time.Time) string
	resolve  func(*ProviderArtist, []byte, time.Time) error
	events   func(*ProviderArtist, []byte, time.Time) error
}

func (s *TicketmasterState) step(tc TicketmasterConfig, c Config, fetch Fetch, persist func(TicketmasterState) error, now time.Time, adapter providerAdapter) (bool, error) {
	if !tc.Enabled || s.Blocked {
		return false, nil
	}
	if err := tc.Validate(); err != nil {
		return false, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), "Ticketmaster", adapter.name))
	}
	if now.Unix() < s.NextHTTP || now.Unix() < s.PauseUntil || len(s.Artists) == 0 {
		return false, nil
	}
	day := now.UTC().Format("2006-01-02")
	if s.BudgetDay == "" || day > s.BudgetDay {
		s.BudgetDay = day
		s.Used = 0
	}
	if s.Used >= tc.DailyBudget || day < s.BudgetDay {
		return false, nil
	}
	s.Cursor = max(0, s.Cursor) % len(s.Artists)
	for n := 0; n < len(s.Artists); n++ {
		idx := (s.Cursor + n) % len(s.Artists)
		a := &s.Artists[idx]
		if now.Unix() < a.Lookup.NextRefresh {
			continue
		}
		if a.Lookup.Page == 0 {
			a.QueryDay = now.UTC().Format("2006-01-02")
		}
		endpoint := adapter.request(a, c, now)
		s.Used++
		s.NextHTTP = now.Unix() + max(1, adapter.interval)
		s.Cursor = (idx + 1) % len(s.Artists)
		a.Lookup.LastAttempt = now.Unix()
		if err := persist(*s); err != nil {
			return false, fmt.Errorf("%s request reservation could not be saved", adapter.name)
		}
		response, err := fetch(endpoint)
		if err != nil {
			s.failure(a, adapter.name+" could not be reached", now, 0)
		} else if response.Status != 200 {
			delay := RetryDelay(response.Headers, now)
			s.failure(a, fmt.Sprintf("%s returned HTTP %d", adapter.name, response.Status), now, delay)
			if response.Status == 429 || response.Status >= 500 {
				s.PauseUntil = a.Lookup.NextRefresh
			}
			if response.Status == 401 || response.Status == 403 {
				s.PauseUntil = now.Add(time.Hour).Unix()
			}
		} else {
			if a.Mapping.ProviderID == "" {
				err = adapter.resolve(a, response.Body, now)
				if err == nil && c.ProviderAutoMatch {
					a.adoptUniqueCandidate()
				}
			} else {
				err = adapter.events(a, response.Body, now)
			}
			if err != nil {
				s.failure(a, err.Error(), now, 0)
			} else {
				a.Lookup.Failures = 0
				a.Lookup.LastError = ""
			}
		}
		if adapter.name == "Skiddle" {
			if response.Status >= 400 && response.Status < 500 && response.Status != 429 {
				s.Blocked = true
				a.Lookup.Outcome = "blocked"
			}
			if a.Lookup.Outcome == "failed" {
				jitter := int64(now.Nanosecond() % 11)
				a.Lookup.NextRefresh += jitter
				s.PauseUntil = max(s.PauseUntil, a.Lookup.NextRefresh)
				if a.Lookup.Failures >= 3 {
					s.Blocked = true
					a.Lookup.Outcome = "blocked"
				}
			} else if a.Lookup.NextRefresh > now.Unix() {
				a.Lookup.NextRefresh += int64(now.Nanosecond() % 60)
			}
		}
		s.Revision++
		if err := persist(*s); err != nil {
			return true, fmt.Errorf("%s results could not be saved", adapter.name)
		}
		return true, nil
	}
	return false, nil
}
func (s *TicketmasterState) failure(a *ProviderArtist, message string, now time.Time, delay int64) {
	a.Lookup.Failures = min(a.Lookup.Failures+1, 7)
	a.Lookup.NextRefresh = now.Unix() + max(int64(30)<<min(a.Lookup.Failures-1, 6), delay)
	a.Lookup.Outcome = "failed"
	a.Lookup.LastError = message
}
func (a *ProviderArtist) resolve(body []byte, now time.Time) error {
	var r struct {
		Page struct {
			Total *int `json:"totalElements"`
		} `json:"page"`
		Embedded struct {
			Attractions []struct {
				ID, Name      string
				ExternalLinks map[string][]struct {
					URL string `json:"url"`
				} `json:"externalLinks"`
			}
		} `json:"_embedded"`
	}
	if json.Unmarshal(body, &r) != nil || r.Page.Total == nil || *r.Page.Total < 0 || *r.Page.Total < len(r.Embedded.Attractions) {
		return fmt.Errorf("invalid Ticketmaster artist search response")
	}
	a.Candidates = nil
	a.CandidatesComplete = *r.Page.Total == len(r.Embedded.Attractions)
	matches := []string{}
	for _, candidate := range r.Embedded.Attractions {
		if !validProviderID(candidate.ID) || normalize(candidate.Name) != normalize(a.Artist.Name) {
			continue
		}
		if len(a.Candidates) < 20 {
			a.Candidates = append(a.Candidates, ArtistCandidate{candidate.ID, candidate.Name})
		}
		for _, links := range candidate.ExternalLinks {
			for _, link := range links {
				u, err := url.Parse(link.URL)
				if err == nil && u.Scheme == "https" && u.Host == "musicbrainz.org" && u.Path == "/artist/"+a.Mapping.MusicBrainzID && ValidMBID(a.Mapping.MusicBrainzID) {
					matches = append(matches, candidate.ID)
				}
			}
		}
	}
	// Exact names alone remain candidates. Adopt only one distinct corroborated ID.
	unique := map[string]bool{}
	for _, id := range matches {
		unique[id] = true
	}
	if len(unique) == 1 && *r.Page.Total == len(r.Embedded.Attractions) {
		for id := range unique {
			a.Mapping.ProviderID = id
		}
		a.Mapping.MatchMethod = "musicbrainz-link"
		a.Lookup.Outcome = "resolved"
		a.Lookup.NextRefresh = 0
	} else {
		a.Lookup.Outcome = "unresolved"
		a.Lookup.NextRefresh = now.Add(7 * 24 * time.Hour).Unix()
	}
	a.Lookup.LastSuccess = now.Unix()
	return nil
}
func (a *ProviderArtist) events(body []byte, now time.Time) error {
	var r tmEvents
	if json.Unmarshal(body, &r) != nil || r.Page.Number == nil || r.Page.Total == nil || r.Page.Pages == nil || *r.Page.Number != a.Lookup.Page || *r.Page.Total < 0 || *r.Page.Pages < 0 {
		return fmt.Errorf("invalid Ticketmaster event pagination")
	}
	if *r.Page.Total > 1000 || *r.Page.Pages > 5 {
		return fmt.Errorf("Ticketmaster results exceed deep pagination limit; cached concerts retained")
	}
	if len(r.Embedded.Events) > 200 || (len(r.Embedded.Events) == 0 && *r.Page.Total > 0) || (*r.Page.Pages > 0 && *r.Page.Number >= *r.Page.Pages) {
		return fmt.Errorf("inconsistent Ticketmaster event page")
	}
	parsed := []Concert{}
	for _, e := range r.Embedded.Events {
		if !validProviderID(e.ID) {
			return fmt.Errorf("invalid Ticketmaster event ID")
		}
		linked := false
		for _, at := range e.Embedded.Attractions {
			if at.ID == a.Mapping.ProviderID {
				linked = true
			}
		}
		if !linked {
			continue
		} // Keywords and title mentions never establish a performer credit.
		at := now.UTC().Format(time.RFC3339)
		event := Concert{InternalID: "ticketmaster:" + e.ID, ArtistIDs: []string{a.Artist.ID}, Title: e.Name, LocalDate: e.Dates.Start.LocalDate, LocalTime: e.Dates.Start.LocalTime, Timezone: e.Dates.Timezone, StartUTC: e.Dates.Start.DateTime, Status: e.Dates.Status.Code, DatePrecision: "unknown", FirstSeenAt: at, LastSeenAt: at}
		if end, _, ok := DateInterval(e.Dates.End.LocalDate); ok && len(e.Dates.End.LocalDate) == 10 {
			if start, _, ok := DateInterval(event.LocalDate); !ok || !end.Before(start) {
				event.EndDate = e.Dates.End.LocalDate
			}
		}
		if event.Status == "canceled" {
			event.Status = "cancelled"
		}
		if event.Status == "" {
			event.Status = "unknown"
		}
		if _, _, ok := DateInterval(event.LocalDate); ok && len(event.LocalDate) == 10 {
			event.DatePrecision = "day"
		} else {
			event.LocalDate = ""
		}
		if e.Dates.Start.DateTBD || e.Dates.Start.DateTBA {
			event.LocalDate = ""
			event.DatePrecision = "unknown"
		}
		if e.Dates.Start.TimeTBA || e.Dates.Start.NoSpecificTime {
			event.LocalTime = ""
			event.StartUTC = ""
		}
		if len(e.Embedded.Venues) > 0 {
			v := e.Embedded.Venues[0]
			event.Venue = v.Name
			event.City = v.City.Name
			event.Region = v.State.StateCode
			event.Country = v.Country.CountryCode
		}
		u := safeWebURL(e.URL)
		if u != "" {
			event.TicketURLs = []string{u}
		}
		event.Sources = []ConcertSource{{Provider: "Ticketmaster", EventID: e.ID, URL: u, FetchedAt: at}}
		parsed = append(parsed, event)
	}
	return a.commitConcertPage(parsed, len(r.Embedded.Events), *r.Page.Total, *r.Page.Pages, now, "Ticketmaster")
}

func (a *ProviderArtist) commitConcertPage(parsed []Concert, rawCount, total, pages int, now time.Time, provider string) error {
	if a.Lookup.Page > 0 && a.ExpectedTotal != total {
		a.Lookup.Page = 0
		a.Pending = nil
		return fmt.Errorf("%s result count changed during pagination; restarting", provider)
	}
	received := a.Received + rawCount
	if a.Lookup.Page == 0 {
		received = rawCount
	}
	if received > total || (a.Lookup.Page+1 >= pages && received != total) {
		a.Lookup.Page = 0
		a.Pending = nil
		return fmt.Errorf("%s event pages incomplete; cached concerts retained", provider)
	}
	seen := map[string]bool{}
	if a.Lookup.Page > 0 {
		for _, event := range a.Pending {
			seen[event.InternalID] = true
		}
	}
	for _, event := range parsed {
		if seen[event.InternalID] {
			a.Lookup.Page = 0
			a.Pending = nil
			return fmt.Errorf("%s repeated an event during pagination; restarting", provider)
		}
		seen[event.InternalID] = true
	}
	if a.Lookup.Page == 0 {
		a.Pending = nil
	}
	a.Pending = append(a.Pending, parsed...)
	a.ExpectedTotal = total
	a.Received = received
	if a.Lookup.Page+1 < pages {
		a.Lookup.Page++
		a.Lookup.NextRefresh = 0
		a.Lookup.Outcome = "fetching"
		return nil
	}
	if total != len(a.Pending) { // Nonmatching attraction credits may be discarded, so only reject impossible overflow.
		if len(a.Pending) > total {
			return fmt.Errorf("%s event count inconsistent", provider)
		}
	}
	old := map[string]Concert{}
	for _, e := range a.Concerts {
		old[e.InternalID] = e
	}
	unique := map[string]Concert{}
	for _, e := range a.Pending {
		if previous, ok := old[e.InternalID]; ok {
			e.FirstSeenAt = previous.FirstSeenAt
		}
		unique[e.InternalID] = e
	}
	a.Concerts = nil
	for _, e := range unique {
		a.Concerts = append(a.Concerts, e)
	}
	sort.Slice(a.Concerts, func(i, j int) bool { return a.Concerts[i].InternalID < a.Concerts[j].InternalID })
	a.Pending = nil
	a.Lookup.Page = 0
	a.Lookup.LastSuccess = now.Unix()
	a.Lookup.NextRefresh = now.Add(24 * time.Hour).Unix()
	a.Lookup.Outcome = "found"
	if len(a.Concerts) == 0 {
		a.Lookup.Outcome = "empty"
		a.Lookup.NextRefresh = now.Add(72 * time.Hour).Unix()
	}
	return nil
}

type tmEvents struct {
	Page struct {
		Number *int `json:"number"`
		Total  *int `json:"totalElements"`
		Pages  *int `json:"totalPages"`
	} `json:"page"`
	Embedded struct {
		Events []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			URL   string `json:"url"`
			Dates struct {
				End struct {
					LocalDate string `json:"localDate"`
				} `json:"end"`
				Timezone string `json:"timezone"`
				Start    struct {
					LocalDate      string `json:"localDate"`
					LocalTime      string `json:"localTime"`
					DateTime       string `json:"dateTime"`
					DateTBD        bool   `json:"dateTBD"`
					DateTBA        bool   `json:"dateTBA"`
					TimeTBA        bool   `json:"timeTBA"`
					NoSpecificTime bool   `json:"noSpecificTime"`
				} `json:"start"`
				Status struct {
					Code string `json:"code"`
				} `json:"status"`
			} `json:"dates"`
			Embedded struct {
				Attractions []struct {
					ID string `json:"id"`
				} `json:"attractions"`
				Venues []struct {
					Name string `json:"name"`
					City struct {
						Name string `json:"name"`
					} `json:"city"`
					State struct {
						StateCode string `json:"stateCode"`
					} `json:"state"`
					Country struct {
						CountryCode string `json:"countryCode"`
					} `json:"country"`
				} `json:"venues"`
			} `json:"_embedded"`
		} `json:"events"`
	} `json:"_embedded"`
}
