package radar

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Concert is independent of any provider's response or the legacy report model.
type Concert struct {
	DoorsTime     string          `json:"doorsOpenTime,omitempty"`
	InternalID    string          `json:"internalId"`
	ArtistIDs     []string        `json:"navidromeArtistIds"`
	Title         string          `json:"title"`
	LocalDate     string          `json:"localStartDate"`
	LocalTime     string          `json:"localStartTime,omitempty"`
	Timezone      string          `json:"timezone,omitempty"`
	StartUTC      string          `json:"startUtc,omitempty"`
	EndDate       string          `json:"endDate,omitempty"`
	DatePrecision string          `json:"datePrecision"`
	Venue         string          `json:"venueName,omitempty"`
	City          string          `json:"city,omitempty"`
	Region        string          `json:"region,omitempty"`
	Country       string          `json:"country,omitempty"`
	TicketURLs    []string        `json:"ticketUrls,omitempty"`
	Status        string          `json:"status"`
	Sources       []ConcertSource `json:"sources"`
	FirstSeenAt   string          `json:"firstSeenAt"`
	LastSeenAt    string          `json:"lastSeenAt"`
}
type ConcertSource struct {
	Provider  string `json:"provider"`
	EventID   string `json:"eventId"`
	URL       string `json:"sourceUrl"`
	FetchedAt string `json:"fetchedAt"`
}
type ArtistMapping struct {
	NavidromeID    string `json:"navidromeArtistId"`
	MusicBrainzID  string `json:"musicBrainzId,omitempty"`
	ProviderID     string `json:"providerArtistId,omitempty"`
	MatchMethod    string `json:"matchMethod"`
	ManualOverride string `json:"manualOverride,omitempty"`
}
type LookupState struct {
	LastAttempt int64  `json:"lastAttemptAt"`
	LastSuccess int64  `json:"lastSuccessAt,omitempty"`
	NextRefresh int64  `json:"nextRefreshAt"`
	Outcome     string `json:"outcome"`
	LastError   string `json:"lastError,omitempty"`
	Failures    int    `json:"failures"`
	Page        int    `json:"paginationPage"`
}
type TicketmasterConfig struct {
	Enabled     bool
	APIKey      string
	DailyBudget int
	Overrides   map[string]string
}

// Avoid compiling a large counted regex: TinyGo's small WASM stack can
// overflow during regexp initialization for this simple ASCII constraint.
func validProviderID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func (c TicketmasterConfig) Validate() error {
	if c.DailyBudget < 1 || c.DailyBudget > 4500 {
		return fmt.Errorf("Ticketmaster budget must be between 1 and 4500")
	}
	if c.Enabled && strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("Ticketmaster needs your free developer API key")
	}
	for id, value := range c.Overrides {
		if id == "" || !validProviderID(value) {
			return fmt.Errorf("invalid Ticketmaster attraction override")
		}
	}
	return nil
}
func safeWebURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}
func (c Concert) reportRecord() Release {
	return Release{ID: c.InternalID, Title: c.Title, Date: c.LocalDate, Type: "Concert", Venue: strings.Join(nonempty(c.Venue, c.City, c.Region, c.Country), ", "), Cancelled: c.Status == "cancelled", Concert: &c}
}
func nonempty(values ...string) []string {
	out := []string{}
	for _, s := range values {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

type providerView struct {
	Name, IDLabel, HTMLID string
	State                 *TicketmasterState
	Budget                int
}

func providerViews(s State, c Config) []providerView {
	out := []providerView{}
	if c.Ticketmaster.Enabled {
		out = append(out, providerView{"Ticketmaster", "attraction", "provider-status", s.Ticketmaster, c.Ticketmaster.DailyBudget})
	}
	if c.SeatGeek.Enabled {
		out = append(out, providerView{"SeatGeek", "performer", "seatgeek-provider-status", s.SeatGeek, c.SeatGeek.DailyBudget})
	}
	if c.Skiddle.Enabled {
		out = append(out, providerView{"Skiddle", "artist", "skiddle-provider-status", s.Skiddle, c.Skiddle.DailyBudget})
	}
	return out
}
func providerEntries(s State, c Config) []Entry {
	entries := append([]Entry(nil), s.Entries...)
	allowed := map[string]bool{}
	for _, e := range s.Entries {
		if IncludedArtist(e.Artist, c) {
			allowed[e.Artist.ID] = true
		}
	}
	for _, provider := range providerViews(s, c) {
		if provider.State == nil {
			continue
		}
		for _, a := range provider.State.Artists {
			if !allowed[a.Artist.ID] {
				continue
			}
			e := Entry{Artist: a.Artist, CheckedAt: time.Unix(a.Lookup.LastSuccess, 0).UTC().Format(time.RFC3339)}
			for _, concert := range a.Concerts {
				e.Releases = append(e.Releases, concert.reportRecord())
			}
			entries = append(entries, e)
		}
	}
	return entries
}

// Include persisted provider revisions in the report view key. A callback can
// crash after committing provider results but before publishing HTML/ICS; the
// next callback must publish those results even when no new request is due.
func ProviderPublicationVersion(s State, c Config) string {
	var out strings.Builder
	for _, provider := range providerViews(s, c) {
		if provider.State == nil {
			continue
		}
		state := provider.State
		fmt.Fprintf(&out, "/%s:%d:%d:%d", provider.Name, state.Revision, state.Used, len(state.Artists))
	}
	return out.String()
}
