package radar

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

//go:embed dashboard.html
var dashboard string

//go:embed dashboard.css
var dashboardCSS string

//go:embed dashboard.js
var dashboardJS string

type agendaShow struct {
	reportRelease
	timing, status, endDate, localTime, country string
}

type agendaArtist struct {
	name, website string
}

func Render(s State, c Config, now time.Time) ([]byte, error) {
	visible := visibleReleases(s, c, now)
	events := calendarEvents(visible, c.EventNamespace(), true)
	return render(s, c, now, visible, events, CalendarICS(events, now))
}

func render(s State, c Config, now time.Time, visible []reportRelease, events []CalendarEvent, ics []byte) ([]byte, error) {
	today := now.UTC().Format(time.DateOnly)
	groups := map[string][]agendaShow{}
	timings := map[string]bool{}
	statuses := map[string]bool{}
	var sourceNames, countries []string
	artistsByID := make(map[string]agendaArtist, len(s.Entries))
	for _, entry := range s.Entries {
		if IncludedArtist(entry.Artist, c) {
			artistsByID[entry.Artist.ID] = agendaArtist{entry.Artist.Name, safeWebURL(entry.ArtistWebsite)}
		}
	}
	for _, r := range visible {
		concert := reportConcert(r)
		show := agendaShow{
			reportRelease: r, timing: "Upcoming", endDate: r.lastDay(),
			localTime: concert.LocalTime, country: countryCode(concert.Country),
		}
		countries = append(countries, show.country)
		switch {
		case len(r.Date) < 10:
			show.timing = "Date uncertain"
		case show.endDate < today:
			show.timing = "Recent"
		case r.Date == today:
			show.timing = "Today"
		case r.Date < today:
			show.timing = "Ongoing"
		}
		switch {
		case r.Cancelled:
			show.status = "Cancelled"
		case r.Concert != nil && r.Concert.Status == "postponed":
			show.status = "Postponed"
		case r.Concert != nil && r.Concert.Status == "rescheduled":
			show.status = "Rescheduled"
		}
		statuses[show.status] = true
		timings[show.timing] = true
		for _, source := range concert.Sources {
			sourceNames = append(sourceNames, source.Provider)
		}
		group := show.timing
		if group == "Today" || group == "Ongoing" {
			group = "Upcoming"
		}
		groups[group] = append(groups[group], show)
	}

	var rows, timingOptions, sourceOptions, countryOptions strings.Builder
	for _, group := range []struct{ name, label string }{
		{"Upcoming", "Upcoming"}, {"Recent", "Recent"}, {"Date uncertain", "Dates to be confirmed"},
	} {
		shows := groups[group.name]
		if len(shows) == 0 {
			continue
		}
		sort.SliceStable(shows, func(i, j int) bool {
			a, b := shows[i], shows[j]
			if a.Date != b.Date {
				if group.name == "Recent" {
					return a.Date > b.Date
				}
				// Place listings without even a partial date at the end.
				return b.Date == "" || (a.Date != "" && a.Date < b.Date)
			}
			if a.localTime != b.localTime {
				return b.localTime == "" || (a.localTime != "" && a.localTime < b.localTime)
			}
			return strings.ToLower(a.ArtistName+a.Title) < strings.ToLower(b.ArtistName+b.Title)
		})
		fmt.Fprintf(&rows, `<section class="agenda-group"><h2>%s</h2><div class="list">`, group.label)
		for _, show := range shows {
			writeAgendaShow(&rows, show, artistsByID)
		}
		rows.WriteString("</div></section>")
		fmt.Fprintf(&timingOptions, `<option value="timing:%s">%s</option>`, group.name, group.label)
	}
	for _, timing := range []string{"Today", "Ongoing"} {
		if timings[timing] {
			fmt.Fprintf(&timingOptions, `<option value="timing:%s">%s</option>`, timing, timing)
		}
	}
	for _, status := range []string{"Cancelled", "Postponed", "Rescheduled"} {
		if statuses[status] {
			fmt.Fprintf(&timingOptions, `<option value="status:%s">%s</option>`, status, status)
		}
	}
	for _, name := range sortedUnique(sourceNames) {
		fmt.Fprintf(&sourceOptions, `<option value="%s">%s</option>`, html.EscapeString(name), html.EscapeString(name))
	}
	for _, code := range sortedUnique(countries) {
		fmt.Fprintf(&countryOptions, `<option value="%s">%s</option>`, code, code)
	}
	countryHidden := ""
	if countryOptions.Len() == 0 {
		countryHidden = "hidden"
	}
	complete, artists, pending, lookupDetails := lookupStatus(s, c, now)
	problems := []string{s.Problem}
	if s.MusicBrainzProblem != "" {
		retry := time.Unix(s.NextHTTP, 0).UTC().Format("2 Jan 2006, 15:04:05 UTC")
		problems = append(problems, s.MusicBrainzProblem+". Checks paused. Automatic retry after "+retry+". Existing results are kept.")
	}
	if s.IntegrationProblem != "" {
		problems = append(problems, "Integration status: "+s.IntegrationProblem)
	}
	problem := ""
	if text := strings.Join(nonempty(problems...), " · "); text != "" {
		problem = `<p class="notice" role="status">` + html.EscapeString(text) + "</p>"
	}

	exportHidden, emptyHidden := "", ""
	if len(events) == 0 {
		exportHidden = "hidden"
	}
	if len(visible) > 0 {
		emptyHidden = "hidden"
	}
	window := fmt.Sprintf("Next %d days", c.AheadDays)
	if c.PastDays > 0 {
		window += fmt.Sprintf(" and previous %d days", c.PastDays)
	}
	theme := c.Theme
	if theme != "bright" && theme != "black" {
		theme = "dark"
	}
	// A single replacement pass keeps inserted metadata from being interpreted as a template.
	result := strings.NewReplacer(
		"@@CSS@@", strings.ReplaceAll(dashboardCSS, "@@SKIDDLE_LOGO@@", skiddleLogoBase64()),
		"@@JS@@", dashboardJS,
		"@@THEME@@", theme,
		"@@GENERATED@@", now.UTC().Format("2 Jan 2006, 15:04 UTC"),
		"@@WINDOW@@", window,
		"@@PERIOD_OPTIONS@@", timingOptions.String(),
		"@@SOURCE_OPTIONS@@", sourceOptions.String(),
		"@@COUNTRY_OPTIONS@@", countryOptions.String(),
		"@@COUNTRY_HIDDEN@@", countryHidden,
		"@@EXPORT_HIDDEN@@", exportHidden,
		"@@CALENDAR_TEXT@@", html.EscapeString(string(ics)),
		"@@ICS@@", base64.StdEncoding.EncodeToString(ics),
		"@@CALENDAR_STATS@@", html.EscapeString(calendarStats(len(visible), len(events))),
		"@@TODAY@@", today,
		"@@COUNT@@", fmt.Sprint(len(visible), " ", pluralConcerts(len(visible))),
		"@@COMPLETE@@", fmt.Sprint(complete),
		"@@ARTISTS@@", fmt.Sprint(artists),
		"@@PENDING@@", fmt.Sprint(pending),
		"@@PROBLEM@@", problem,
		"@@ROWS@@", rows.String(),
		"@@ISSUES@@", lookupDetails,
		"@@EMPTYHIDDEN@@", emptyHidden,
	).Replace(dashboard)
	return []byte(result), nil
}

func pluralConcerts(count int) string {
	if count == 1 {
		return "concert"
	}
	return "concerts"
}

func writeAgendaShow(rows *strings.Builder, show agendaShow, artistsByID map[string]agendaArtist) {
	esc := html.EscapeString
	sources := orderedConcertSources(reportConcert(show.reportRelease).Sources)
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Provider)
	}
	fmt.Fprintf(rows, `<article class="concert" data-timing="%s" data-status="%s" data-date="%s" data-end-date="%s" data-sources="%s" data-country="%s">`,
		show.timing, show.status, esc(show.Date), esc(show.endDate), esc(strings.Join(sortedUnique(names), "|")), show.country)
	if date, _, ok := DateInterval(show.Date); ok && len(show.Date) == 10 {
		fmt.Fprintf(rows, `<time class="concert-date" datetime="%s" aria-label="%s"><span class="date-day">%s</span><span>%s</span></time>`,
			esc(show.Date), date.Format("2 January 2006"), date.Format("2"), date.Format("Jan 2006"))
	} else {
		fmt.Fprintf(rows, `<div class="concert-date">%s</div>`, esc(DateLabel(show.Date)))
	}
	rows.WriteString(`<div class="concert-body"><div class="artist-line">`)
	writeAgendaArtists(rows, show, artistsByID)
	if show.status != "" {
		fmt.Fprintf(rows, `<span class="event-status" data-alert="%t">%s</span>`, show.status != "Rescheduled", show.status)
	} else if show.timing == "Today" || show.timing == "Ongoing" {
		fmt.Fprintf(rows, `<span class="event-status">%s</span>`, show.timing)
	}
	rows.WriteString("</div><h3>")
	title := esc(show.Title)
	for _, source := range sources {
		if address := safeWebURL(source.URL); address != "" {
			title = fmt.Sprintf(`<a href="%s" rel="noreferrer">%s</a>`, esc(address), title)
			break
		}
	}
	rows.WriteString(title)
	rows.WriteString("</h3>")
	if show.Venue != "" {
		fmt.Fprintf(rows, `<p class="venue">%s</p>`, esc(show.Venue))
	}
	if details := concertDetails(show.Release); details != "" {
		fmt.Fprintf(rows, `<p class="concert-details">%s</p>`, esc(details))
	}
	rows.WriteString(concertSourcesHTML(sources))
	if checked, err := time.Parse(time.RFC3339, show.CheckedAt); err == nil && checked.Unix() > 0 {
		fmt.Fprintf(rows, `<p class="checked-at">Checked <time datetime="%s" title="%s">%s</time></p>`,
			esc(show.CheckedAt), checked.UTC().Format("2 Jan 2006, 15:04 UTC"), checked.UTC().Format("2 Jan"))
	}
	rows.WriteString("</div></article>")
}

func writeAgendaArtists(rows *strings.Builder, show agendaShow, artistsByID map[string]agendaArtist) {
	websites := make(map[string]string, len(show.ArtistIDs))
	for _, id := range show.ArtistIDs {
		artist := artistsByID[id]
		if previous, exists := websites[artist.name]; exists && previous != artist.website {
			websites[artist.name] = ""
		} else {
			websites[artist.name] = artist.website
		}
	}

	rows.WriteString(`<span class="artist">`)
	for i, name := range show.ArtistNames {
		if i > 0 {
			rows.WriteString(" / ")
		}
		if website := websites[name]; website != "" {
			fmt.Fprintf(rows, `<a href="%s" rel="noreferrer">%s</a>`, html.EscapeString(website), html.EscapeString(name))
		} else {
			rows.WriteString(html.EscapeString(name))
		}
	}
	rows.WriteString("</span>")
}

func lookupStatus(s State, c Config, now time.Time) (complete, artists, pending int, details string) {
	var issues strings.Builder
	issueCount := 0
	for _, e := range s.Entries {
		if !IncludedArtist(e.Artist, c) {
			continue
		}
		artists++
		switch e.Phase {
		case "done":
			complete++
		case "match", "releases":
			pending++
		}
		if e.Problem != "" {
			issueCount++
			fmt.Fprintf(&issues, `<li><strong>%s</strong> · Navidrome ID <code>%s</code><br>%s</li>`,
				html.EscapeString(e.Artist.Name), html.EscapeString(e.Artist.ID), html.EscapeString(e.Problem))
		}
	}
	if issueCount > 0 {
		label := fmt.Sprintf("MusicBrainz issues (%d)", issueCount)
		details = "<details><summary>" + label + "</summary><ul>" + issues.String() + "</ul></details>"
	}
	return complete, artists, pending, details + providerStatusHTML(s, c, now)
}
