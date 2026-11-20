package radar

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
	"time"
)

//go:embed skiddle-logo.png
var skiddleLogo []byte

const SkiddleLogoURL = "https://d1plawd8huk6hh.cloudfront.net/assets/logo/png/skiddle-logo-black-landscape.png"

func skiddleLogoBase64() string { return base64.StdEncoding.EncodeToString(skiddleLogo) }

func concertDetails(r Release) string {
	if r.Concert == nil {
		return "Source: MusicBrainz"
	}
	e := r.Concert
	details := []string{"Status: " + e.Status}
	if e.EndDate != "" && e.EndDate != e.LocalDate {
		details = append(details, "Through "+DateLabel(e.EndDate))
	}
	if e.DoorsTime != "" {
		details = append(details, "Doors open: "+e.DoorsTime)
	}
	if e.LocalTime != "" {
		details = append(details, "Local time: "+e.LocalTime+" "+e.Timezone)
	}
	for _, source := range e.Sources {
		details = append(details, "Source: "+source.Provider)
	}
	return strings.Join(details, " · ")
}
func providerStatusHTML(s State, c Config, now time.Time) string {
	var b strings.Builder
	for _, provider := range providerViews(s, c) {
		fmt.Fprintf(&b, `<details class="notice provider-status" id="%s"><summary>%s lookup status</summary>`, provider.HTMLID, provider.Name)
		if provider.State == nil {
			b.WriteString(`<p>Provider state unavailable. No lookup has been confirmed.</p></details>`)
			continue
		}
		if provider.Name == "Skiddle" {
			b.WriteString(`<p>Data from Skiddle <span class="skiddle-logo" role="img" aria-label="Skiddle logo"></span></p>`)
		}
		tm := provider.State
		if tm.Blocked {
			b.WriteString(`<p>Lookups stopped after errors. Fix the configuration, then use manual refresh.</p>`)
		}
		used := tm.Used
		if now.UTC().Format("2006-01-02") > tm.BudgetDay {
			used = 0
		}
		fmt.Fprintf(&b, "<p>%d / %d locally reserved requests today (UTC). Other applications may share the provider quota.</p>", used, provider.Budget)
		if used >= provider.Budget {
			b.WriteString(`<p>Daily budget reached. Cached results remain visible; requests resume on a later UTC day.</p>`)
		}
		if tm.PauseUntil > now.Unix() {
			fmt.Fprintf(&b, "<p>Provider requests paused until %s UTC.</p>", time.Unix(tm.PauseUntil, 0).UTC().Format("2 Jan 15:04"))
		}
		b.WriteString("<ul>")
		allowed := map[string]bool{}
		for _, e := range s.Entries {
			if IncludedArtist(e.Artist, c) {
				allowed[e.Artist.ID] = true
			}
		}
		completed, queued, unresolved := 0, 0, 0
		seen := map[string]bool{}
		for _, artist := range tm.Artists {
			if !allowed[artist.Artist.ID] {
				continue
			}
			seen[artist.Artist.ID] = true
			switch artist.Lookup.Outcome {
			case "found", "empty":
				completed++
			case "unresolved":
				if c.ProviderAutoMatch && artist.Lookup.NextRefresh == 0 && !artist.CandidatesComplete && len(artist.Candidates) == 1 {
					queued++
				} else {
					unresolved++
				}
			default:
				queued++
			}
		}
		queued += len(allowed) - len(seen)
		fmt.Fprintf(&b, "<li class=\"meta\">%d concert lookups complete · %d queued or retrying · %d identities unresolved</li>", completed, queued, unresolved)
		for _, a := range tm.Artists {
			if !allowed[a.Artist.ID] {
				continue
			}
			outcome := map[string]string{"": "Waiting for lookup", "found": "Concerts found", "empty": "Checked: no concerts found", "unresolved": "Artist identity unresolved; add a " + provider.IDLabel + " ID mapping", "blocked": "Lookup blocked; fix configuration and use manual refresh", "failed": "Lookup failed; cached results retained", "fetching": "Fetching remaining pages; cached results retained", "resolved": "Identity resolved; concert lookup queued"}[a.Lookup.Outcome]
			if outcome == "" {
				outcome = "Waiting for lookup"
			}
			if c.ProviderAutoMatch && a.Lookup.Outcome == "unresolved" && a.Lookup.NextRefresh == 0 && !a.CandidatesComplete && len(a.Candidates) == 1 {
				outcome = "Artist identity unresolved; rechecking cached candidate"
			}
			fmt.Fprintf(&b, "<li><strong>%s</strong> · Navidrome ID <code>%s</code> — %s", html.EscapeString(a.Artist.Name), html.EscapeString(a.Artist.ID), html.EscapeString(outcome))
			if a.Lookup.LastError != "" {
				fmt.Fprintf(&b, ". %s", html.EscapeString(a.Lookup.LastError))
			}
			if a.Mapping.MatchMethod == "unique-exact-name" {
				b.WriteString("<br>Artist match: single exact-name candidate")
			}
			for _, candidate := range a.Candidates {
				fmt.Fprintf(&b, "<br>Candidate: %s · %s ID <code>%s</code>", html.EscapeString(candidate.Name), provider.IDLabel, html.EscapeString(candidate.ID))
			}
			if a.Lookup.LastSuccess > 0 {
				fmt.Fprintf(&b, "<br>Last successful response: %s UTC", time.Unix(a.Lookup.LastSuccess, 0).UTC().Format("2 Jan 2006 15:04"))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></details>")
	}
	return b.String()
}

func concertSourcesHTML(r Release) string {
	if r.Concert == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<span class="concert-sources">`)
	for _, source := range r.Concert.Sources {
		if u := safeWebURL(source.URL); u != "" {
			class := ""
			if source.Provider == "Skiddle" {
				class = "skiddle-source"
			}
			fmt.Fprintf(&b, `<a class="%s" href="%s" rel="noreferrer" aria-label="%s listing">`, class, html.EscapeString(u), html.EscapeString(source.Provider))
			if source.Provider == "Skiddle" {
				b.WriteString(`<span class="skiddle-logo" role="img" aria-label="Skiddle logo"></span>`)
			}
			fmt.Fprintf(&b, `<span>%s listing</span></a>`, html.EscapeString(source.Provider))
		}
	}
	b.WriteString(`</span>`)
	return b.String()
}
