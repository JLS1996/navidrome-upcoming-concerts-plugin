package radar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type CalendarEvent struct {
	EndDate     string `json:"endDate,omitempty"`
	Status      string `json:"status,omitempty"`
	Key         string `json:"key"`
	UID         string `json:"uid"`
	Summary     string `json:"summary"`
	Date        string `json:"date"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (e CalendarEvent) Fingerprint() string {
	return digest(e.Summary + "\x00" + e.Date + "\x00" + e.Description + "\x00" + e.URL + "\x00" + e.Status + "\x00" + e.EndDate)
}
func (r Release) ValidID() bool {
	if ValidMBID(r.ID) {
		return true
	}
	if r.Concert == nil {
		return false
	}
	if id, ok := strings.CutPrefix(r.ID, "ticketmaster:"); ok {
		return validProviderID(id)
	}
	if id, ok := strings.CutPrefix(r.ID, "skiddle:"); ok {
		return validSeatGeekID(id)
	}
	if id, ok := strings.CutPrefix(r.ID, "seatgeek:"); ok {
		return validSeatGeekID(id)
	}
	return false
}
func (r Release) SourceURL() string {
	if r.Concert != nil {
		for _, source := range r.Concert.Sources {
			if u := safeWebURL(source.URL); u != "" {
				return u
			}
		}
		return ""
	}
	if r.ValidID() {
		return "https://musicbrainz.org/event/" + r.ID
	}
	return ""
}
func Events(s State, c Config, namespace string, now time.Time, exactOnly bool) []CalendarEvent {
	result := []CalendarEvent{}
	for _, r := range visibleReleases(s, c, now) {
		if exactOnly && len(r.Date) != 10 {
			continue
		}
		endDate := ""
		if r.Concert != nil {
			endDate = r.Concert.EndDate
		}
		status := ""
		if r.Cancelled {
			status = "CANCELLED"
		}
		key := r.ID
		description := r.TypeLabel() + " concert. " + r.Venue + ". Source dates may change. " + DateLabel(r.Date)
		if r.Concert != nil {
			description += ". Status: " + r.Concert.Status
			if r.Concert.EndDate != "" && r.Concert.EndDate != r.Date {
				description += ". Through " + DateLabel(r.Concert.EndDate)
			}
			for _, source := range r.Concert.Sources {
				description += ". Source: " + source.Provider + " " + source.URL
				if source.Provider == "Skiddle" {
					description += ". Skiddle logo: " + SkiddleLogoURL
				}
			}
			if r.Concert.DoorsTime != "" {
				description += ". Doors open: " + r.Concert.DoorsTime
			}
			if r.Concert.LocalTime != "" {
				description += ". Local time: " + r.Concert.LocalTime + " " + r.Concert.Timezone
			}
		}
		result = append(result, CalendarEvent{EndDate: endDate, Status: status, Key: key, UID: "rr-" + digest(namespace+"\x00"+key) + "@concert-radar", Summary: r.ArtistName + " — " + r.Title, Date: r.Date, Description: description, URL: r.SourceURL()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UID < result[j].UID })
	return result
}
func icsEscape(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", ",", "\\,", ";", "\\;").Replace(s)
}

// Fold by UTF-8 octets without breaking a code point; continuation starts with a space.
func foldICS(s string) string {
	var out strings.Builder
	limit := 75
	for len(s) > limit {
		n := limit
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		out.WriteString(s[:n])
		out.WriteString("\r\n ")
		s = s[n:]
		limit = 74
	}
	out.WriteString(s)
	out.WriteString("\r\n")
	return out.String()
}
func EventICS(e CalendarEvent, now time.Time) string {
	t, _, valid := DateInterval(e.Date)
	if !valid || len(e.Date) != 10 {
		return ""
	}
	end := t.AddDate(0, 0, 1)
	if last, _, ok := DateInterval(e.EndDate); ok && len(e.EndDate) == 10 && !last.Before(t) {
		end = last.AddDate(0, 0, 1)
	}
	lines := []string{"BEGIN:VEVENT", "UID:" + icsEscape(e.UID), "DTSTAMP:" + now.UTC().Format("20060102T150405Z"), "DTSTART;VALUE=DATE:" + t.Format("20060102"), "DTEND;VALUE=DATE:" + end.Format("20060102"), "SUMMARY:" + icsEscape(e.Summary), "DESCRIPTION:" + icsEscape(e.Description), "URL:" + icsEscape(e.URL), "TRANSP:TRANSPARENT", "END:VEVENT"}
	if e.Status == "CANCELLED" {
		lines = append(lines[:len(lines)-1], "STATUS:CANCELLED", "END:VEVENT")
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(foldICS(l))
	}
	return b.String()
}
func CalendarICS(events []CalendarEvent, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//navidrome-upcoming-concerts-plugin//Concerts//EN\r\nCALSCALE:GREGORIAN\r\nX-WR-CALNAME:Concerts\r\n")
	for _, e := range events {
		b.WriteString(EventICS(e, now))
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}
func ownedICS(body []byte, uid string) bool {
	s := strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n ", ""), "\r\n\t", "")
	return (strings.Contains(s, "PRODID:-//navidrome-upcoming-concerts-plugin//") || strings.Contains(s, "PRODID:-//Release Radar//")) && strings.Contains(s, "\r\nUID:"+uid+"\r\n")
}
func FullCycle(s State, c Config) bool {
	for _, provider := range providerViews(s, c) {
		if provider.State == nil {
			return false
		}
		allowed := map[string]bool{}
		for _, e := range s.Entries {
			if IncludedArtist(e.Artist, c) {
				allowed[e.Artist.ID] = true
			}
		}
		for _, a := range provider.State.Artists {
			if allowed[a.Artist.ID] {
				switch a.Lookup.Outcome {
				case "found", "empty", "unresolved":
				default:
					return false
				}
				delete(allowed, a.Artist.ID)
			}
		}
		if len(allowed) > 0 {
			return false
		}
	}
	if c.ExcludeUncredited && !c.DirectReady {
		return false
	}
	for _, e := range s.Entries {
		if !IncludedArtist(e.Artist, c) {
			continue
		}
		if e.Phase == "match" || e.Phase == "releases" {
			return false
		}
	}
	return true
}
func CalendarStats(s State, c Config, namespace string, now time.Time) string {
	all := len(Events(s, c, namespace, now, false))
	exact := len(Events(s, c, namespace, now, true))
	eventWord, releaseWord := "events", "concerts"
	if exact == 1 {
		eventWord = "event"
	}
	if all-exact == 1 {
		releaseWord = "concert"
	}
	return fmt.Sprintf("%d dated %s; %d %s with uncertain dates excluded from calendar export", exact, eventWord, all-exact, releaseWord)
}
