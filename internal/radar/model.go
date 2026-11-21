package radar

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const Version = "0.5.0"

func MusicBrainzUserAgent() string {
	return "navidrome-upcoming-concerts-plugin/" + Version + " (https://github.com/JLS1996/navidrome-upcoming-concerts-plugin)"
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidMBID(s string) bool { return uuidRE.MatchString(s) }

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MBID string `json:"musicBrainzId"`
}

// Release retains the v0.3 cache wire format so upgrading does not discard scans.
// Live providers use Concert records; this legacy name is not a release feature.
type Release struct {
	Concert   *Concert `json:"concert,omitempty"`
	Venue     string   `json:"venue,omitempty"`
	Cancelled bool     `json:"cancelled,omitempty"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Date      string   `json:"first-release-date"`
	EndDate   string   `json:"endDate,omitempty"`
	Type      string   `json:"primary-type"`
	Secondary []string `json:"secondary-types"`
}
type Entry struct {
	Artist       Artist    `json:"artist"`
	ResolvedMBID string    `json:"resolvedMbid,omitempty"`
	Phase        string    `json:"phase"`
	Offset       int       `json:"offset"`
	Pending      []Release `json:"pending,omitempty"`
	Releases     []Release `json:"releases,omitempty"`
	CheckedAt    string    `json:"checkedAt,omitempty"`
	Problem      string    `json:"problem,omitempty"`
	RetryAt      int64     `json:"retryAt,omitempty"`
	Failures     int       `json:"failures,omitempty"`
}
type State struct {
	LookupConfig           string             `json:"lookupConfig,omitempty"`
	Skiddle                *SkiddleState      `json:"-"`
	SeatGeek               *SeatGeekState     `json:"-"`
	Ticketmaster           *TicketmasterState `json:"-"`
	Direct                 CreditInventory    `json:"directCredits,omitempty"`
	MusicBrainzFailures    int                `json:"musicBrainzFailures,omitempty"`
	MusicBrainzProblem     string             `json:"musicBrainzProblem,omitempty"`
	IntegrationProblem     string             `json:"integrationProblem,omitempty"`
	RefreshToggle          *bool              `json:"refreshToggle,omitempty"`
	ProviderRefreshPending uint8              `json:"providerRefreshPending,omitempty"`
	RefreshPending         bool               `json:"refreshPending,omitempty"`
	NextDiscovery          int64              `json:"nextDiscovery,omitempty"`
	RenderedAt             int64              `json:"renderedAt"`
	ViewConfig             string             `json:"viewConfig"`
	Schema                 int                `json:"schema"`
	DiscoveredAt           int64              `json:"discoveredAt"`
	Scope                  string             `json:"scope"`
	Entries                []Entry            `json:"entries"`
	Cursor                 int                `json:"cursor"`
	NextHTTP               int64              `json:"nextHttp"`
	Problem                string             `json:"problem,omitempty"`
}
type Config struct {
	ProviderAutoMatch   bool
	Skiddle             SkiddleConfig
	SeatGeek            SeatGeekConfig
	DeduplicateConcerts bool
	Ticketmaster        TicketmasterConfig
	Theme               string
	ExcludeUncredited   bool
	DirectReady         bool
	DirectCredits       map[string]bool
	CalendarNamespace   string
	Username            string
	AheadDays           int
	PastDays            int
	AutoMatch           bool
	Overrides           map[string]string
	Exclusions          map[string]bool
}

func DefaultConfig() Config {
	return Config{ProviderAutoMatch: true, Skiddle: SkiddleConfig{DailyBudget: 500, Overrides: map[string]string{}}, DeduplicateConcerts: true, SeatGeek: SeatGeekConfig{DailyBudget: 1000, Overrides: map[string]string{}}, Ticketmaster: TicketmasterConfig{DailyBudget: 4000, Overrides: map[string]string{}}, Theme: "dark", CalendarNamespace: "navidrome-upcoming-concerts-plugin", AheadDays: 90, PastDays: 0, AutoMatch: true, Overrides: map[string]string{}, Exclusions: map[string]bool{}}
}

func (c Config) Validate() error {
	if c.AheadDays < 1 || c.AheadDays > 730 || c.PastDays < 0 || c.PastDays > 365 {
		return fmt.Errorf("invalid date window")
	}
	for id, mbid := range c.Overrides {
		if id == "" || !ValidMBID(mbid) {
			return fmt.Errorf("invalid artist override: use Navidrome artist IDs mapped to MusicBrainz UUIDs")
		}
	}
	return nil
}
func (c Config) Fingerprint() string {
	b, _ := json.Marshal(struct {
		User       string
		Overrides  map[string]string
		Exclusions map[string]bool
		Auto       bool
	}{c.Username, c.Overrides, c.Exclusions, c.AutoMatch})
	return string(b)
}

// ApplyRefresh treats both saved transitions as one refresh request. The first
// value establishes a baseline so upgrades do not restart a completed catalog.
func (s *State) ApplyRefresh(value bool) bool {
	if s.RefreshToggle == nil {
		s.RefreshToggle = &value
		return false
	}
	if *s.RefreshToggle == value {
		return false
	}
	s.RefreshToggle = &value
	s.RefreshPending = true
	s.NextDiscovery = 0
	return true
}
func (s *State) Reconcile(artists []Artist, c Config, scope string, now time.Time) {
	old := map[string]Entry{}
	// Never retain another user's or another library selection's results.
	if s.Scope == scope {
		for _, e := range s.Entries {
			old[e.Artist.ID] = e
		}
	}
	entries := []Entry{}
	seen := map[string]bool{}
	for _, a := range artists {
		if a.ID == "" || seen[a.ID] || c.Exclusions[a.ID] {
			continue
		}
		seen[a.ID] = true
		mbid := a.MBID
		if override := c.Overrides[a.ID]; override != "" {
			mbid = override
		}
		if !ValidMBID(mbid) {
			mbid = ""
		}
		e, ok := old[a.ID]
		if !ok || a.Name != e.Artist.Name || mbid != e.Artist.MBID {
			e = Entry{}
		}
		// Keep unfinished work during routine discovery. A manual refresh resets retries.
		if !s.RefreshPending && ok && a.Name == e.Artist.Name && mbid == e.Artist.MBID && (e.Phase == "releases" || e.Phase == "match" && c.AutoMatch) {
			a.MBID = mbid
			e.Artist = a
			entries = append(entries, e)
			continue
		}
		a.MBID = mbid
		e.Artist = a
		if mbid != "" {
			e.ResolvedMBID = mbid
		}
		if e.ResolvedMBID != "" {
			e.Phase = "releases"
		} else if c.AutoMatch {
			e.Phase = "match"
		} else {
			e.Phase = "unresolved"
			e.Problem = "No MusicBrainz ID; add a manual override."
		}
		e.Offset = 0
		e.Pending = nil
		e.Failures = 0
		e.RetryAt = 0
		if e.Phase != "unresolved" {
			e.Problem = ""
		}
		entries = append(entries, e)
	}
	s.Schema = 1
	s.Scope = scope
	s.Entries = entries
	s.Cursor = 0
	s.DiscoveredAt = now.Unix()
	s.RefreshPending = false
	s.NextDiscovery = 0
	s.Problem = ""
}
func ParseArtists(raw string) ([]Artist, error) {
	var envelope struct {
		Response struct {
			Status string `json:"status"`
			Error  struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
			Artists *struct {
				Index []struct {
					Artist []Artist `json:"artist"`
				} `json:"index"`
			} `json:"artists"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, err
	}
	if envelope.Response.Status != "ok" {
		// Navidrome uses this exact code/message for an empty, already-verified library.
		if envelope.Response.Error.Code == 70 && envelope.Response.Error.Message == "Library not found or empty" {
			return []Artist{}, nil
		}
		return nil, fmt.Errorf("Navidrome: %s", envelope.Response.Error.Message)
	}
	if envelope.Response.Artists == nil {
		return nil, fmt.Errorf("Navidrome artist list is missing")
	}
	out := []Artist{}
	for _, idx := range envelope.Response.Artists.Index {
		out = append(out, idx.Artist...)
	}
	return out, nil
}

// DateInterval preserves partial dates, and rejects malformed dates instead of guessing.
func DateInterval(s string) (time.Time, time.Time, bool) {
	var layout string
	switch len(s) {
	case 4:
		layout = "2006"
	case 7:
		layout = "2006-01"
	case 10:
		layout = "2006-01-02"
	default:
		return time.Time{}, time.Time{}, false
	}
	start, err := time.Parse(layout, s)
	if err != nil || start.Year() < 1000 {
		return time.Time{}, time.Time{}, false
	}
	end := start
	switch len(s) {
	case 4:
		end = start.AddDate(1, 0, 0).AddDate(0, 0, -1)
	case 7:
		end = start.AddDate(0, 1, 0).AddDate(0, 0, -1)
	}
	return start, end, true
}
func DateLabel(s string) string {
	t, _, ok := DateInterval(s)
	if !ok {
		return "Date unknown"
	}
	switch len(s) {
	case 4:
		return t.Format("2006") + " (day unknown)"
	case 7:
		return t.Format("January 2006") + " (day unknown)"
	default:
		return t.Format("2 January 2006")
	}
}
func (r Release) PrimaryType() string {
	if r.Type == "" {
		return "Other"
	}
	return r.Type
}
func (r Release) TypeLabel() string {
	label := r.PrimaryType()
	if len(r.Secondary) > 0 {
		label += " · " + strings.Join(r.Secondary, ", ")
	}
	return label
}

type reportRelease struct {
	ArtistIDs, ArtistNames []string
	Release
	ArtistName, CheckedAt string
}

// Shared release groups appear once, with stable combined artist credits in
// HTML, calendar exports and notifications. Prefer the newest committed metadata.
func visibleReleases(s State, c Config, now time.Time) []reportRelease {
	if c.ExcludeUncredited && !c.DirectReady {
		return nil
	}
	groupArtists := map[string][]Artist{}
	groups := map[string]reportRelease{}
	credits := map[string]map[string]bool{}
	for _, e := range providerEntries(s, c) {
		if !IncludedArtist(e.Artist, c) {
			continue
		}
		for _, r := range e.Releases {
			if !r.ValidID() {
				continue
			}
			groupArtists[r.ID] = append(groupArtists[r.ID], e.Artist)
			previous, exists := groups[r.ID]
			if !exists || e.CheckedAt > previous.CheckedAt {
				groups[r.ID] = reportRelease{Release: r, CheckedAt: e.CheckedAt}
			}
			if credits[r.ID] == nil {
				credits[r.ID] = map[string]bool{}
			}
			credits[r.ID][e.Artist.Name] = true
		}
	}
	ids := []string{}
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []reportRelease{}
	for _, id := range ids {
		r := groups[id]
		names := []string{}
		for name := range credits[id] {
			names = append(names, name)
		}
		sort.Strings(names)
		r.ArtistName = strings.Join(names, " / ")
		r.ArtistNames = append([]string(nil), names...)
		for _, artist := range groupArtists[id] {
			r.ArtistIDs = append(r.ArtistIDs, artist.ID)
		}
		r.ArtistIDs = sortedUnique(r.ArtistIDs)
		out = append(out, r)
	}
	if c.DeduplicateConcerts {
		out = deduplicateReports(out)
	}
	visible := []reportRelease{}
	for _, event := range out {
		if Visible(event.Release, c, now) {
			visible = append(visible, event)
		}
	}
	return visible
}

// Extend a known start date only with a valid, exact end date.
func (r Release) dateRange() (start, end time.Time, ok bool) {
	start, end, ok = DateInterval(r.Date)
	if endDate := r.endDate(); ok && len(r.Date) == 10 && len(endDate) == 10 {
		if last, _, valid := DateInterval(endDate); valid && !last.Before(start) {
			end = last
		}
	}
	return
}

func (r Release) endDate() string {
	if r.Concert != nil {
		return r.Concert.EndDate
	}
	return r.EndDate
}

func (r Release) lastDay() string {
	if _, end, ok := r.dateRange(); ok && len(r.Date) == 10 {
		return end.Format(time.DateOnly)
	}
	return ""
}

func Visible(r Release, c Config, now time.Time) bool {
	if r.Cancelled && r.Concert == nil {
		return false
	}
	if r.Type == "Concert" {
		if r.Date == "" && r.Concert != nil {
			return true
		}
		start, end, ok := r.dateRange()
		if !ok {
			return false
		}
		today := now.UTC().Truncate(24 * time.Hour)
		return !end.Before(today.AddDate(0, 0, -c.PastDays)) && !start.After(today.AddDate(0, 0, c.AheadDays))
	}
	return false
}

func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Keep existing calendar IDs stable across the project rename.
func (c Config) EventNamespace() string {
	namespace := c.CalendarNamespace
	if namespace == "navidrome-upcoming-concerts-plugin" {
		namespace = "concert-radar"
	}
	return namespace + "\x00" + c.Username
}
