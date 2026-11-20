package radar

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SeatGeek uses the same durable queue schema in its own file and budget.
type SeatGeekState = TicketmasterState
type SeatGeekConfig struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	DailyBudget  int
	Overrides    map[string]string
}

func validSeatGeekID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}
func (c SeatGeekConfig) QueueConfig() TicketmasterConfig {
	return TicketmasterConfig{Enabled: c.Enabled, APIKey: c.ClientID, DailyBudget: c.DailyBudget, Overrides: c.Overrides}
}
func (c SeatGeekConfig) Validate() error {
	if c.DailyBudget < 1 || c.DailyBudget > 4500 {
		return fmt.Errorf("SeatGeek local budget must be between 1 and 4500")
	}
	if c.Enabled && strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("SeatGeek needs your developer client ID")
	}
	for id, value := range c.Overrides {
		if id == "" || !validSeatGeekID(value) {
			return fmt.Errorf("SeatGeek mappings must contain positive numeric performer IDs")
		}
	}
	return nil
}

var seatGeekBase = "https://api.seatgeek.com/2/" // Only build-tagged tests override this.
func (s *TicketmasterState) StepSeatGeek(sc SeatGeekConfig, c Config, fetch Fetch, persist func(SeatGeekState) error, now time.Time) (bool, error) {
	if err := sc.Validate(); err != nil {
		return false, err
	}
	return s.step(sc.QueueConfig(), c, fetch, persist, now, providerAdapter{name: "SeatGeek", request: func(a *ProviderArtist, c Config, now time.Time) string {
		q := url.Values{"client_id": {sc.ClientID}, "per_page": {"200"}, "page": {strconv.Itoa(a.Lookup.Page + 1)}, "taxonomies.name": {"concert"}}
		if sc.ClientSecret != "" {
			q.Set("client_secret", sc.ClientSecret)
		}
		path := "events"
		if a.Mapping.ProviderID == "" {
			path = "performers"
			q.Set("q", a.Artist.Name)
		} else {
			q.Set("performers.id", a.Mapping.ProviderID)
			q.Set("sort", "datetime_local.asc")
			today := providerQueryDay(a, now)
			q.Set("datetime_local.gte", today.AddDate(0, 0, -c.PastDays).Format("2006-01-02"))
			q.Set("datetime_local.lt", today.AddDate(0, 0, c.AheadDays+1).Format("2006-01-02"))
		}
		return seatGeekBase + path + "?" + q.Encode()
	}, resolve: (*ProviderArtist).resolveSeatGeek, events: (*ProviderArtist).seatGeekEvents})
}

type sgMeta struct {
	Page    *int `json:"page"`
	Total   *int `json:"total"`
	PerPage *int `json:"per_page"`
}

func (m sgMeta) validate(page, count int) error {
	if m.Page == nil || m.Total == nil || m.PerPage == nil || *m.Page != page+1 || *m.Total < 0 || *m.PerPage < 1 || *m.PerPage > 200 || count > *m.PerPage {
		return fmt.Errorf("invalid SeatGeek pagination")
	}
	if count == 0 && *m.Total > 0 {
		return fmt.Errorf("incomplete SeatGeek page")
	}
	return nil
}
func (a *ProviderArtist) resolveSeatGeek(body []byte, now time.Time) error {
	var r struct {
		Meta       sgMeta `json:"meta"`
		Performers []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"performers"`
	}
	if json.Unmarshal(body, &r) != nil {
		return fmt.Errorf("invalid SeatGeek performer response")
	}
	if err := r.Meta.validate(0, len(r.Performers)); err != nil {
		return err
	}
	a.Candidates = nil
	a.CandidatesComplete = *r.Meta.Total == len(r.Performers)
	for _, p := range r.Performers {
		if p.ID > 0 && normalize(p.Name) == normalize(a.Artist.Name) && len(a.Candidates) < 20 {
			a.Candidates = append(a.Candidates, ArtistCandidate{ID: strconv.FormatInt(p.ID, 10), Name: p.Name})
		}
	}
	// SeatGeek's documented links do not establish a MusicBrainz identity.
	// Require an explicit performer-ID mapping rather than adopting a name alone.
	a.Lookup.Outcome = "unresolved"
	a.Lookup.LastSuccess = now.Unix()
	a.Lookup.NextRefresh = now.Add(7 * 24 * time.Hour).Unix()
	return nil
}

type sgEvent struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	Type          string `json:"type"`
	DateTimeLocal string `json:"datetime_local"`
	DateTimeUTC   string `json:"datetime_utc"`
	TimeTBD       bool   `json:"time_tbd"`
	DateTBD       bool   `json:"date_tbd"`
	DateTimeTBD   bool   `json:"datetime_tbd"`
	Status        string `json:"status"`
	Performers    []struct {
		ID int64 `json:"id"`
	} `json:"performers"`
	Taxonomies []struct {
		Name string `json:"name"`
	} `json:"taxonomies"`
	Venue struct{ Name, City, State, Country, Timezone string } `json:"venue"`
}

func (a *ProviderArtist) seatGeekEvents(body []byte, now time.Time) error {
	var r struct {
		Meta   sgMeta    `json:"meta"`
		Events []sgEvent `json:"events"`
	}
	if json.Unmarshal(body, &r) != nil {
		return fmt.Errorf("invalid SeatGeek event response")
	}
	if err := r.Meta.validate(a.Lookup.Page, len(r.Events)); err != nil {
		return err
	}
	pages := (*r.Meta.Total + *r.Meta.PerPage - 1) / *r.Meta.PerPage
	if *r.Meta.Total > 1000 || pages > 5 {
		return fmt.Errorf("SeatGeek results exceed local pagination safety limit; cached concerts retained")
	}
	if (*r.Meta.Total == 0 && a.Lookup.Page != 0) || (*r.Meta.Total > 0 && a.Lookup.Page >= pages) {
		return fmt.Errorf("inconsistent SeatGeek page")
	}
	parsed := []Concert{}
	for _, e := range r.Events {
		if e.ID <= 0 {
			return fmt.Errorf("invalid SeatGeek event ID")
		}
		linked := false
		for _, p := range e.Performers {
			if strconv.FormatInt(p.ID, 10) == a.Mapping.ProviderID {
				linked = true
			}
		}
		music := e.Type == "concert"
		for _, t := range e.Taxonomies {
			if t.Name == "concert" {
				music = true
			}
		}
		if !linked || !music {
			continue
		}
		at := now.UTC().Format(time.RFC3339)
		id := strconv.FormatInt(e.ID, 10)
		concert := Concert{InternalID: "seatgeek:" + id, ArtistIDs: []string{a.Artist.ID}, Title: e.Title, Venue: e.Venue.Name, City: e.Venue.City, Region: e.Venue.State, Country: e.Venue.Country, Timezone: e.Venue.Timezone, Status: e.Status, DatePrecision: "unknown", FirstSeenAt: at, LastSeenAt: at}
		if concert.Status == "" {
			concert.Status = "unknown"
		}
		if concert.Status == "canceled" {
			concert.Status = "cancelled"
		}
		if !e.DateTBD && !e.DateTimeTBD {
			if local, err := time.Parse("2006-01-02T15:04:05", e.DateTimeLocal); err == nil {
				concert.LocalDate = local.Format("2006-01-02")
				concert.DatePrecision = "day"
				if !e.TimeTBD {
					concert.LocalTime = local.Format("15:04:05")
				}
			}
			if !e.TimeTBD {
				raw := e.DateTimeUTC
				if !strings.HasSuffix(raw, "Z") {
					raw += "Z"
				}
				if utc, err := time.Parse(time.RFC3339, raw); err == nil {
					concert.StartUTC = utc.UTC().Format(time.RFC3339)
				}
			}
		}
		u := seatGeekURL(e.URL)
		if u != "" {
			concert.TicketURLs = []string{u}
		}
		concert.Sources = []ConcertSource{{Provider: "SeatGeek", EventID: id, URL: u, FetchedAt: at}}
		parsed = append(parsed, concert)
	}
	return a.commitConcertPage(parsed, len(r.Events), *r.Meta.Total, pages, now, "SeatGeek")
}
func seatGeekURL(raw string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		raw = "https://seatgeek.com" + raw
	}
	return safeWebURL(raw)
}
