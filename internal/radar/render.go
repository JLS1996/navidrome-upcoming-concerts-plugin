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

type row struct{ Artist, Title, Type, TypeLabel, Secondary, Date, DateLabel, URL, Status, CheckedAt, Details, Sources, EndDate string }
type issue struct{ Name, ID, Message string }
type report struct {
	Generated                  string
	Artists, Complete, Pending int
	Rows                       []row
	Issues                     []issue
	Problem                    string
	AheadDays, PastDays        int
}

func Render(s State, c Config, now time.Time) ([]byte, error) {
	visible := visibleReleases(s, c, now)
	events := calendarEvents(visible, c.EventNamespace(), true)
	return render(s, c, now, visible, events, CalendarICS(events, now))
}

func render(s State, c Config, now time.Time, visible []reportRelease, events []CalendarEvent, ics []byte) ([]byte, error) {
	p := report{Generated: now.UTC().Format("2 Jan 2006, 15:04 UTC"), Artists: 0, Problem: s.Problem, AheadDays: c.AheadDays, PastDays: c.PastDays}
	today := now.UTC().Format("2006-01-02")
	for _, e := range s.Entries {
		if !IncludedArtist(e.Artist, c) {
			continue
		}
		p.Artists++
		if e.Phase == "done" {
			p.Complete++
		} else if e.Phase == "match" || e.Phase == "releases" {
			p.Pending++
		}
		if e.Problem != "" {
			p.Issues = append(p.Issues, issue{e.Artist.Name, e.Artist.ID, e.Problem})
		}
	}
	for _, r := range visible {
		endDate := ""
		_, end, _ := r.dateRange()
		if len(r.Date) == 10 {
			endDate = end.Format("2006-01-02")
		}
		status := "Upcoming"
		if len(r.Date) < 10 {
			status = "Date uncertain"
		} else if end.Format("2006-01-02") < today {
			status = "Recent"
		} else if r.Date == today {
			status = "Today"
		} else if r.Date < today && end.Format("2006-01-02") >= today {
			status = "Ongoing"
		}
		if r.Cancelled {
			status = "Cancelled"
		} else if r.Concert != nil && r.Concert.Status == "postponed" {
			status = "Postponed"
		} else if r.Concert != nil && r.Concert.Status == "rescheduled" {
			status = "Rescheduled"
		}
		p.Rows = append(p.Rows, row{Details: concertDetails(r.Release), Sources: concertSourcesHTML(r.Release), EndDate: endDate, Artist: r.ArtistName, Title: r.Title + venueSuffix(r.Venue), Type: r.PrimaryType(), TypeLabel: r.TypeLabel(), Secondary: strings.Join(r.Secondary, "|"), Date: r.Date, DateLabel: DateLabel(r.Date), URL: r.SourceURL(), Status: status, CheckedAt: r.CheckedAt})
	}
	sort.SliceStable(p.Rows, func(i, j int) bool {
		a, b := p.Rows[i], p.Rows[j]
		if a.Date == b.Date {
			return strings.ToLower(a.Artist+a.Title) < strings.ToLower(b.Artist+b.Title)
		}
		return a.Date < b.Date
	})
	// Avoid text/template reflection, unsupported by TinyGo 0.40.1 at runtime.
	esc := html.EscapeString
	var rows, issues strings.Builder
	for _, r := range p.Rows {
		checked := r.CheckedAt
		if t, err := time.Parse(time.RFC3339, checked); err == nil {
			checked = t.UTC().Format("2 Jan 2006, 15:04 UTC")
		}
		title := esc(r.Title)
		if r.URL != "" {
			title = fmt.Sprintf(`<a href="%s" rel="noreferrer">%s</a>`, esc(r.URL), title)
		}
		fmt.Fprintf(&rows, `<article class="concert" data-type="%s" data-secondary="%s" data-period="%s" data-date="%s" data-end-date="%s"><div class="concert-date date">%s<br><span class="meta">%s</span></div><div class="concert-body"><div class="artist">%s</div><h2>%s</h2><div class="meta">Last checked:<br><time>%s</time></div></div><span class="tag">%s</span><div class="meta concert-details">%s%s</div></article>`, esc(r.Type), esc(r.Secondary), esc(r.Status), esc(r.Date), esc(r.EndDate), esc(r.DateLabel), esc(r.Status), esc(r.Artist), title, esc(checked), esc(r.TypeLabel), esc(r.Details), r.Sources)
	}
	if len(p.Issues) > 0 {
		summary := fmt.Sprintf("%d artists need attention", len(p.Issues))
		if len(p.Issues) == 1 {
			summary = "1 artist needs attention"
		}
		fmt.Fprintf(&issues, `<details><summary>%s</summary><ul>`, summary)
		for _, i := range p.Issues {
			fmt.Fprintf(&issues, `<li><strong>%s</strong> · Navidrome ID <code>%s</code><br>%s</li>`, esc(i.Name), esc(i.ID), esc(i.Message))
		}
		issues.WriteString("</ul></details>")
	}
	if s.MusicBrainzProblem != "" {
		retry := time.Unix(s.NextHTTP, 0).UTC().Format("2 Jan 2006, 15:04:05 UTC")
		status := s.MusicBrainzProblem + ". Checks paused; automatic retry after " + retry + ". Existing results are kept."
		if p.Problem != "" {
			p.Problem += " · "
		}
		p.Problem += status
	}
	problem := ""
	if s.IntegrationProblem != "" {
		if p.Problem != "" {
			p.Problem += " · "
		}
		p.Problem += "Integration status: " + s.IntegrationProblem
	}
	if p.Problem != "" {
		problem = `<div class="notice" role="status">` + esc(p.Problem) + `</div>`
	}
	hidden := ""
	if len(p.Rows) > 0 {
		hidden = "hidden"
	}
	emptyNote := ""
	if len(events) == 0 {
		emptyNote = `<p class="notice">No dated calendar events yet. Importing this calendar will add nothing. Checks may still be running, or no exact concert dates fall within your filters.</p>`
	}
	var typeOptions, periodOptions strings.Builder
	availableTypes, availablePeriods := map[string]bool{}, map[string]bool{}
	for _, r := range p.Rows {
		availableTypes[r.Type] = true
		for _, tag := range strings.Split(r.Secondary, "|") {
			if tag != "" {
				availableTypes[tag] = true
			}
		}
		availablePeriods[r.Status] = true
	}
	if availableTypes["Concert"] {
		typeOptions.WriteString(`<option value="Concert">Concert</option>`)
	}
	for _, period := range []string{"Upcoming", "Today", "Ongoing", "Recent", "Date uncertain", "Cancelled", "Postponed", "Rescheduled"} {
		if availablePeriods[period] {
			fmt.Fprintf(&periodOptions, `<option value="%s">%s</option>`, period, period)
		}
	}
	// NewReplacer makes a single pass: inserted metadata is never interpreted as a template.
	theme := c.Theme
	if theme != "bright" && theme != "black" {
		theme = "dark"
	}
	result := strings.NewReplacer("@@SKIDDLE_LOGO@@", skiddleLogoBase64(), "@@THEME@@", theme, "@@PERIOD_OPTIONS@@", periodOptions.String(), "@@TYPE_OPTIONS@@", typeOptions.String(), "@@CALENDAR_EMPTY_NOTE@@", emptyNote, "@@CALENDAR_TEXT@@", esc(string(ics)), "@@ICS@@", base64.StdEncoding.EncodeToString(ics), "@@CALENDAR_STATS@@", esc(calendarStats(len(visible), len(events))), "@@TODAY@@", now.UTC().Format("2006-01-02"), "@@COUNT@@", fmt.Sprint(len(p.Rows)), "@@COMPLETE@@", fmt.Sprint(p.Complete), "@@ARTISTS@@", fmt.Sprint(p.Artists), "@@PENDING@@", fmt.Sprint(p.Pending), "@@PAST@@", fmt.Sprint(p.PastDays), "@@AHEAD@@", fmt.Sprint(p.AheadDays), "@@GENERATED@@", esc(p.Generated), "@@PROBLEM@@", problem+providerStatusHTML(s, c, now), "@@ROWS@@", rows.String(), "@@ISSUES@@", issues.String(), "@@EMPTYHIDDEN@@", hidden).Replace(dashboard)
	return []byte(result), nil
}
