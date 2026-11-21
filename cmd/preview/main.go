// preview generates deterministic, explicitly labelled sample data. It makes no network requests.
package main

import (
	"flag"
	"os"
	"time"

	"github.com/JLS1996/navidrome-upcoming-concerts-plugin/internal/radar"
)

func main() {
	providers := flag.Bool("providers", false, "include fictional Ticketmaster records and lookup outcomes")
	theme := flag.String("theme", "dark", "dashboard theme: bright, dark or black")
	out := flag.String("out", "dist/preview.html", "output path")
	flag.Parse()
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	c := radar.DefaultConfig()
	c.Theme = *theme
	c.Username = "demo"
	c.PastDays = 14
	checked := now.Format(time.RFC3339)
	s := radar.State{Schema: 1, Problem: "Demo report with fictional concerts.", Entries: []radar.Entry{
		{Artist: radar.Artist{ID: "sample-1", Name: "Northbound"}, Phase: "done", CheckedAt: checked, Releases: []radar.Release{
			{ID: "11111111-1111-4111-8111-111111111111", Title: "A Place in the Morning", Date: "2026-12-04", Type: "Concert", Venue: "Demo Hall, London"},
			{ID: "22222222-2222-4222-8222-222222222222", Title: "Slow Light", Date: "2026-11-26", Type: "Concert", Venue: "Sample Arena, Berlin"},
			{ID: "44444444-4444-4444-8444-444444444444", Title: "Winter Residency", Date: "2026-11-29", EndDate: "2026-12-02", Type: "Concert", Venue: "The Lantern, Brighton"},
		}},
		{Artist: radar.Artist{ID: "sample-2", Name: "Glass Orchard"}, Phase: "done", CheckedAt: checked, Releases: []radar.Release{
			{ID: "33333333-3333-4333-8333-333333333333", Title: "Signals / After Hours", Date: "2026-12", Type: "Concert"},
			{ID: "55555555-5555-4555-8555-555555555555", Title: "Marigold Sessions", Date: "2026-11-19", Type: "Concert", Venue: "Café Marigold, Paris"},
			{ID: "66666666-6666-4666-8666-666666666666", Title: "Glasshouse Live", Date: "2026-11-16", Type: "Concert", Venue: "Sample Room, Dublin"},
		}},
		{Artist: radar.Artist{ID: "sample-3", Name: "Ambiguous sample artist"}, Phase: "unresolved", Problem: "Multiple artists share this name. Set an artist ID override."},
	}}
	if *providers {
		c.Ticketmaster.Enabled = true
		s.Ticketmaster = &radar.TicketmasterState{Schema: 1, BudgetDay: now.Format("2006-01-02"), Used: 3, Artists: []radar.ProviderArtist{
			{Artist: s.Entries[0].Artist, Lookup: radar.LookupState{Outcome: "found", LastSuccess: now.Unix()}, Concerts: []radar.Concert{{InternalID: "ticketmaster:demo", ArtistIDs: []string{s.Entries[0].Artist.ID}, Title: "Winter Tour", LocalDate: "2026-12-09", LocalTime: "19:30:00", Timezone: "Europe/London", DatePrecision: "day", Venue: "Demo theatre", City: "London", Country: "GB", Status: "rescheduled", Sources: []radar.ConcertSource{{Provider: "Ticketmaster", EventID: "demo", URL: "https://www.ticketmaster.com/event/demo"}}}}},
			{Artist: s.Entries[1].Artist, Lookup: radar.LookupState{Outcome: "empty", LastSuccess: now.Unix()}},
			{Artist: s.Entries[2].Artist, Lookup: radar.LookupState{Outcome: "unresolved"}, Candidates: []radar.ArtistCandidate{{ID: "fictional-attraction", Name: "Ambiguous sample artist"}}},
		}}
	}
	if *providers {
		c.SeatGeek.Enabled = true
		s.SeatGeek = &radar.SeatGeekState{Schema: 1, BudgetDay: now.Format("2006-01-02"), Used: 1, Artists: []radar.ProviderArtist{{Artist: s.Entries[0].Artist, Lookup: radar.LookupState{Outcome: "found", LastSuccess: now.Unix()}, Concerts: []radar.Concert{{InternalID: "seatgeek:123", ArtistIDs: []string{s.Entries[0].Artist.ID}, Title: "Winter Tour", LocalDate: "2026-12-09", LocalTime: "19:30:00", Timezone: "Europe/London", DatePrecision: "day", Venue: "Demo theatre", City: "London", Country: "GB", Status: "rescheduled", Sources: []radar.ConcertSource{{Provider: "SeatGeek", EventID: "123", URL: "https://seatgeek.com/fictional-tour/123"}}}}}}}
		artist := &s.SeatGeek.Artists[0]
		artist.Concerts = append(artist.Concerts, radar.Concert{
			InternalID: "seatgeek:124", Title: "Acoustic Sessions", LocalDate: "2026-11-17", Venue: "Demo theatre", City: "London", Country: "GB",
			DatePrecision: "day", Status: "cancelled", Sources: []radar.ConcertSource{{Provider: "SeatGeek", EventID: "124", URL: "https://seatgeek.com/fictional-tour/124"}},
		}, radar.Concert{
			InternalID: "seatgeek:125", Title: "Spring Sessions", DatePrecision: "unknown",
			Status: "unknown", Sources: []radar.ConcertSource{{Provider: "SeatGeek", EventID: "125"}},
		})
	}
	if *providers {
		c.Skiddle.Enabled = true
		s.Skiddle = &radar.SkiddleState{Schema: 1, BudgetDay: now.Format("2006-01-02"), Used: 1, Artists: []radar.ProviderArtist{{Artist: s.Entries[0].Artist, Lookup: radar.LookupState{Outcome: "found", LastSuccess: now.Unix()}, Concerts: []radar.Concert{{InternalID: "skiddle:789", ArtistIDs: []string{s.Entries[0].Artist.ID}, Title: "Winter Tour", LocalDate: "2026-12-09", DoorsTime: "19:00", DatePrecision: "day", Venue: "Demo theatre", City: "London", Country: "GB", Status: "unknown", Sources: []radar.ConcertSource{{Provider: "Skiddle", EventID: "789", URL: "https://www.skiddle.com/whats-on/fictional/789/"}}}}}}}
	}
	b, err := radar.Render(s, c, now)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(*out, b, 0644); err != nil {
		panic(err)
	}
}
