package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SkiddleState = TicketmasterState
type SkiddleConfig struct {
	Enabled     bool
	APIKey      string
	DailyBudget int
	Overrides   map[string]string
}

func (c SkiddleConfig) QueueConfig() TicketmasterConfig {
	return TicketmasterConfig{Enabled: c.Enabled, APIKey: c.APIKey, DailyBudget: c.DailyBudget, Overrides: c.Overrides}
}
func (c SkiddleConfig) Validate() error {
	if c.DailyBudget < 1 || c.DailyBudget > 4500 {
		return fmt.Errorf("Skiddle local budget must be between 1 and 4500")
	}
	if c.Enabled && strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("Skiddle needs your developer API key")
	}
	for id, value := range c.Overrides {
		if id == "" || !validSeatGeekID(value) {
			return fmt.Errorf("Skiddle mappings must contain positive numeric artist IDs")
		}
	}
	return nil
}

var skiddleBase = "https://www.skiddle.com/api/v1/" // Build-tagged fixture only.
func (s *TicketmasterState) StepSkiddle(sc SkiddleConfig, c Config, fetch Fetch, persist func(SkiddleState) error, now time.Time) (bool, error) {
	if err := sc.Validate(); err != nil {
		return false, err
	}
	return s.step(sc.QueueConfig(), c, fetch, persist, now, providerAdapter{name: "Skiddle", interval: 3, request: func(a *ProviderArtist, c Config, now time.Time) string {
		q := url.Values{"api_key": {sc.APIKey}, "limit": {"100"}, "offset": {strconv.Itoa(a.Lookup.Page * 100)}}
		path := "events/search/"
		if a.Mapping.ProviderID == "" {
			path = "artists/"
			q.Set("name", a.Artist.Name)
		} else {
			q.Set("a", a.Mapping.ProviderID)
			q.Set("description", "1")
			q.Set("order", "date")
			today := providerQueryDay(a, now)
			q.Set("minDate", today.AddDate(0, 0, -c.PastDays).Format("2006-01-02"))
			q.Set("maxDate", today.AddDate(0, 0, c.AheadDays).Format("2006-01-02"))
		}
		return skiddleBase + path + "?" + q.Encode()
	}, resolve: (*ProviderArtist).resolveSkiddle, events: (*ProviderArtist).skiddleEvents})
}

// Skiddle responses commonly encode identifiers and counts as JSON strings.
type wireInt int64

func (v *wireInt) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if bytes.HasPrefix(data, []byte(`"`)) {
		if json.Unmarshal(data, &raw) != nil {
			return fmt.Errorf("invalid numeric value")
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid numeric value")
	}
	*v = wireInt(n)
	return nil
}

type wireBool bool

func (v *wireBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `" `) {
	case "1", "true":
		*v = true
	case "0", "false", "null", "":
		*v = false
	default:
		return fmt.Errorf("invalid boolean")
	}
	return nil
}

type skiddleEnvelope struct {
	Error   *wireInt        `json:"error"`
	Total   *wireInt        `json:"totalcount"`
	Results json.RawMessage `json:"results"`
}

func parseSkiddle(body []byte) (skiddleEnvelope, error) {
	var r skiddleEnvelope
	if json.Unmarshal(body, &r) != nil || r.Error == nil || *r.Error != 0 || r.Total == nil || *r.Total < 0 {
		return r, fmt.Errorf("invalid Skiddle response or provider error")
	}
	return r, nil
}
func (a *ProviderArtist) resolveSkiddle(body []byte, now time.Time) error {
	r, err := parseSkiddle(body)
	if err != nil {
		return err
	}
	var results []struct {
		ID       wireInt `json:"id"`
		ArtistID wireInt `json:"artistid"`
		Name     string  `json:"name"`
	}
	if json.Unmarshal(r.Results, &results) != nil || len(results) > 100 || int64(len(results)) > int64(*r.Total) || (len(results) == 0 && *r.Total > 0) {
		return fmt.Errorf("invalid Skiddle artist response")
	}
	a.Candidates = nil
	a.CandidatesComplete = int64(*r.Total) == int64(len(results))
	for _, artist := range results {
		id := artist.ID
		if id == 0 {
			id = artist.ArtistID
		}
		if id > 0 && normalize(artist.Name) == normalize(a.Artist.Name) && len(a.Candidates) < 20 {
			a.Candidates = append(a.Candidates, ArtistCandidate{ID: strconv.FormatInt(int64(id), 10), Name: artist.Name})
		}
	}
	a.Lookup.LastSuccess = now.Unix()
	a.Lookup.Outcome = "unresolved"
	a.Lookup.NextRefresh = now.Add(7 * 24 * time.Hour).Unix()
	return nil
}

type skiddleEvent struct {
	ID        wireInt  `json:"id"`
	Name      string   `json:"eventname"`
	Date      string   `json:"date"`
	StartDate string   `json:"startdate"`
	EndDate   string   `json:"enddate"`
	Link      string   `json:"link"`
	Cancelled wireBool `json:"cancelled"`
	Artists   []struct {
		ID         wireInt `json:"artistid"`
		FallbackID wireInt `json:"id"`
	} `json:"artists"`
	Country      string                                            `json:"country"`
	Venue        struct{ Name, Town, Countrycode, Country string } `json:"venue"`
	OpeningTimes struct {
		DoorsOpen string `json:"doorsopen"`
	} `json:"openingtimes"`
}

func (a *ProviderArtist) skiddleEvents(body []byte, now time.Time) error {
	r, err := parseSkiddle(body)
	if err != nil {
		return err
	}
	if *r.Total > 500 {
		return fmt.Errorf("Skiddle results exceed local five-page safety limit; cached concerts retained")
	}
	var results []skiddleEvent
	if json.Unmarshal(r.Results, &results) != nil || len(results) > 100 {
		return fmt.Errorf("invalid Skiddle concert response")
	}
	total := int(*r.Total)
	pages := (total + 99) / 100
	if (len(results) == 0 && total > 0) || (total == 0 && a.Lookup.Page != 0) || (total > 0 && a.Lookup.Page >= pages) {
		return fmt.Errorf("incomplete Skiddle page")
	}
	parsed := []Concert{}
	for _, e := range results {
		if e.ID <= 0 {
			return fmt.Errorf("invalid Skiddle event ID")
		}
		if e.Artists == nil {
			return fmt.Errorf("Skiddle did not supply artist credits; cached concerts retained")
		}
		linked := false
		for _, artist := range e.Artists {
			id := artist.ID
			if id == 0 {
				id = artist.FallbackID
			}
			if strconv.FormatInt(int64(id), 10) == a.Mapping.ProviderID {
				linked = true
			}
		}
		if !linked {
			continue
		}
		id := strconv.FormatInt(int64(e.ID), 10)
		at := now.UTC().Format(time.RFC3339)
		event := Concert{InternalID: "skiddle:" + id, ArtistIDs: []string{a.Artist.ID}, Title: e.Name, Venue: e.Venue.Name, City: e.Venue.Town, Status: "unknown", DatePrecision: "unknown", FirstSeenAt: at, LastSeenAt: at}
		for _, location := range []string{e.Venue.Countrycode, e.Venue.Country, e.Country} {
			if event.Country = countryCode(location); event.Country != "" {
				break
			}
		}
		if e.Cancelled {
			event.Status = "cancelled"
		}
		if _, _, ok := DateInterval(e.Date); ok && len(e.Date) == 10 {
			event.LocalDate = e.Date
			event.DatePrecision = "day"
		}
		if start, err := time.Parse(time.RFC3339, e.StartDate); err == nil {
			event.LocalDate = start.Format("2006-01-02")
			event.LocalTime = start.Format("15:04:05")
			event.StartUTC = start.UTC().Format(time.RFC3339)
			event.DatePrecision = "day"
		}
		end := e.EndDate
		if len(end) > 10 {
			end = end[:10]
		}
		if endDay, _, ok := DateInterval(end); ok && len(end) == 10 {
			if start, _, ok := DateInterval(event.LocalDate); ok && !endDay.Before(start) {
				event.EndDate = end
			}
		}
		// Doors-open time is not advertised as the performance start time.
		event.DoorsTime = e.OpeningTimes.DoorsOpen
		u := safeWebURL(e.Link)
		if u != "" {
			event.TicketURLs = []string{u}
		}
		event.Sources = []ConcertSource{{Provider: "Skiddle", EventID: id, URL: u, FetchedAt: at}}
		parsed = append(parsed, event)
	}
	return a.commitConcertPage(parsed, len(results), total, pages, now, "Skiddle")
}
