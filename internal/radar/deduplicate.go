package radar

import (
	"net/url"
	"sort"
	"strings"
	"unicode"
)

func matchText(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}
func eventURL(raw string) string {
	u, err := url.Parse(safeWebURL(raw))
	if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
		return ""
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	q := u.Query()
	for key := range q {
		if strings.HasPrefix(strings.ToLower(key), "utm_") || key == "fbclid" || key == "gclid" || key == "aid" || key == "rid" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func reportConcert(r reportRelease) Concert {
	if r.Concert != nil {
		c := *r.Concert
		return c
	}
	return Concert{InternalID: r.ID, Title: r.Title, LocalDate: r.Date, EndDate: r.EndDate, Venue: r.Venue, Status: "unknown", Sources: []ConcertSource{{Provider: "MusicBrainz", EventID: r.ID, URL: r.SourceURL(), FetchedAt: r.CheckedAt}}}
}
func providersOf(c Concert) map[string]bool {
	out := map[string]bool{}
	for _, s := range c.Sources {
		out[s.Provider] = true
	}
	return out
}
func sameShow(a, b reportRelease) bool {
	ac, bc := reportConcert(a), reportConcert(b)
	for provider := range providersOf(ac) {
		if providersOf(bc)[provider] {
			return false
		}
	}
	shared := false
	for _, id := range a.ArtistIDs {
		for _, other := range b.ArtistIDs {
			if id == other {
				shared = true
			}
		}
	}
	if !shared {
		return false
	}
	if ac.LocalDate == bc.LocalDate && ac.LocalTime != "" && bc.LocalTime != "" && ac.LocalTime != bc.LocalTime {
		return false
	}
	urls := map[string]bool{}
	for _, s := range ac.Sources {
		if u := eventURL(s.URL); u != "" {
			urls[u] = true
		}
	}
	for _, u := range ac.TicketURLs {
		if u = eventURL(u); u != "" {
			urls[u] = true
		}
	}
	for _, s := range bc.Sources {
		if u := eventURL(s.URL); u != "" && urls[u] {
			return true
		}
	}
	for _, u := range bc.TicketURLs {
		if u = eventURL(u); u != "" && urls[u] {
			return true
		}
	}
	return len(ac.LocalDate) == 10 && ac.LocalDate == bc.LocalDate && ac.Venue != "" && ac.City != "" && ac.Country != "" && matchText(ac.Venue) == matchText(bc.Venue) && matchText(ac.City) == matchText(bc.City) && matchText(ac.Country) == matchText(bc.Country) && (ac.Region == "" || bc.Region == "" || matchText(ac.Region) == matchText(bc.Region)) && (ac.EndDate == "" || bc.EndDate == "" || ac.EndDate == bc.EndDate)
}

// Merge only complete, unambiguous cross-provider groups. A same-day matinee
// and evening show plus a source with no time remain separate, rather than guessing.
func deduplicateReports(rows []reportRelease) []reportRelease {
	buckets := map[string][]int{}
	add := func(key string, index int) { buckets[key] = append(buckets[key], index) }
	for i, r := range rows {
		c := reportConcert(r)
		for _, artist := range r.ArtistIDs {
			if len(c.LocalDate) == 10 && c.Venue != "" && c.City != "" && c.Country != "" {
				add("place:"+strings.Join([]string{artist, c.LocalDate, matchText(c.Venue), matchText(c.City), matchText(c.Country)}, "\x00"), i)
			}
			for _, s := range c.Sources {
				if u := eventURL(s.URL); u != "" {
					add("url:"+artist+"\x00"+u, i)
				}
			}
			for _, raw := range c.TicketURLs {
				if u := eventURL(raw); u != "" {
					add("url:"+artist+"\x00"+u, i)
				}
			}
		}
	}
	edges := make([]map[int]bool, len(rows))
	for _, bucket := range buckets {
		unique := map[int]bool{}
		for _, i := range bucket {
			unique[i] = true
		}
		if len(unique) > 64 {
			continue
		} // Bound work and preserve ambiguous large clusters.
		ids := []int{}
		for i := range unique {
			ids = append(ids, i)
		}
		sort.Ints(ids)
		for x, i := range ids {
			for _, j := range ids[x+1:] {
				if sameShow(rows[i], rows[j]) {
					if edges[i] == nil {
						edges[i] = map[int]bool{}
					}
					if edges[j] == nil {
						edges[j] = map[int]bool{}
					}
					edges[i][j] = true
					edges[j][i] = true
				}
			}
		}
	}
	seen := map[int]bool{}
	out := []reportRelease{}
	for i := range rows {
		if seen[i] {
			continue
		}
		component := []int{i}
		seen[i] = true
		for at := 0; at < len(component); at++ {
			for j := range edges[component[at]] {
				if !seen[j] {
					seen[j] = true
					component = append(component, j)
				}
			}
		}
		sort.Ints(component)
		complete := true
		for _, idx := range component {
			if len(edges[idx]) != len(component)-1 {
				complete = false
			}
		}
		if !complete {
			for _, idx := range component {
				out = append(out, rows[idx])
			}
			continue
		}
		merged := rows[component[0]]
		for _, idx := range component[1:] {
			merged = mergeReports(merged, rows[idx])
		}
		out = append(out, merged)
	}
	return out
}
func sortedUnique(values []string) []string {
	m := map[string]bool{}
	for _, v := range values {
		if v != "" {
			m[v] = true
		}
	}
	out := []string{}
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func mergeReports(a, b reportRelease) reportRelease {
	ac, bc := reportConcert(a), reportConcert(b)
	best, other := ac, bc
	if b.CheckedAt > a.CheckedAt {
		best, other = bc, ac
		a.CheckedAt = b.CheckedAt
	}
	if best.Status == "" || best.Status == "unknown" {
		best.Status = other.Status
	}
	if best.DoorsTime == "" {
		best.DoorsTime = other.DoorsTime
	}
	if best.Venue == "" {
		best.Venue = other.Venue
	}
	if best.City == "" {
		best.City = other.City
	}
	if best.Country == "" {
		best.Country = other.Country
	}
	if best.Region == "" {
		best.Region = other.Region
	}
	if best.LocalTime == "" && best.LocalDate == other.LocalDate {
		best.LocalTime = other.LocalTime
		best.Timezone = other.Timezone
	}
	if best.EndDate == "" && best.LocalDate == other.LocalDate {
		best.EndDate = other.EndDate
	}
	best.InternalID = a.ID
	sources := map[string]ConcertSource{}
	for _, s := range append(append([]ConcertSource(nil), ac.Sources...), bc.Sources...) {
		key := s.Provider + "\x00" + s.EventID + "\x00" + s.URL
		if prev, ok := sources[key]; !ok || s.FetchedAt > prev.FetchedAt {
			sources[key] = s
		}
	}
	keys := []string{}
	for k := range sources {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	best.Sources = nil
	for _, k := range keys {
		best.Sources = append(best.Sources, sources[k])
	}
	best.TicketURLs = sortedUnique(append(append([]string(nil), ac.TicketURLs...), bc.TicketURLs...))
	a.ArtistIDs = sortedUnique(append(a.ArtistIDs, b.ArtistIDs...))
	a.ArtistNames = sortedUnique(append(a.ArtistNames, b.ArtistNames...))
	best.ArtistIDs = append([]string(nil), a.ArtistIDs...)
	a.ArtistName = strings.Join(a.ArtistNames, " / ")
	a.Release = best.reportRecord()
	return a
}
