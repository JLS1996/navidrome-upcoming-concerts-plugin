package radar

import (
	"strings"
	"testing"
)

func duplicateFixture() (State, Config) {
	s := ready()
	s.Entries[0].Phase = "done"
	c := cfg()
	c.Ticketmaster = tmConfig()
	c.SeatGeek = sgConfig()
	base := Concert{Title: "Show", LocalDate: "2026-12-01", LocalTime: "19:30:00", Venue: "Hall", City: "London", Country: "GB", Status: "onsale"}
	tm := base
	tm.InternalID = "ticketmaster:show"
	tm.Sources = []ConcertSource{{Provider: "Ticketmaster", EventID: "show", URL: "https://www.ticketmaster.com/event/show"}}
	sg := base
	sg.InternalID = "seatgeek:1"
	sg.Sources = []ConcertSource{{Provider: "SeatGeek", EventID: "1", URL: "https://seatgeek.com/show/1"}}
	s.Ticketmaster = &TicketmasterState{Schema: 1, Artists: []ProviderArtist{{Artist: s.Entries[0].Artist, Concerts: []Concert{tm}}}}
	s.SeatGeek = &SeatGeekState{Schema: 1, Artists: []ProviderArtist{{Artist: s.Entries[0].Artist, Concerts: []Concert{sg}}}}
	return s, c
}
func TestDeduplicationSettingAppliesToAllOutputs(t *testing.T) {
	s, c := duplicateFixture()
	rows := visibleReleases(s, c, testNow)
	if len(rows) != 1 || len(rows[0].Concert.Sources) != 2 {
		t.Fatal(rows)
	}
	events := Events(s, c, c.EventNamespace(), testNow, true)
	if len(events) != 1 || !strings.Contains(events[0].Description, "Ticketmaster") || !strings.Contains(events[0].Description, "SeatGeek") {
		t.Fatal(events)
	}
	html, _ := Render(s, c, testNow)
	if strings.Count(string(html), `<article class="concert"`) != 1 || !strings.Contains(string(html), "SeatGeek listing") || !strings.Contains(string(html), "Ticketmaster listing") {
		t.Fatal("missing merged attribution")
	}
	if len(s.Ticketmaster.Artists[0].Concerts[0].Sources) != 1 {
		t.Fatal("dedup mutated provider cache")
	}
	c.DeduplicateConcerts = false
	if len(Events(s, c, c.EventNamespace(), testNow, true)) != 2 {
		t.Fatal("setting ignored")
	}
}
func TestDeduplicationKeepsDifferentAndAmbiguousShows(t *testing.T) {
	s, c := duplicateFixture()
	s.SeatGeek.Artists[0].Concerts[0].LocalTime = "21:00:00"
	if len(visibleReleases(s, c, testNow)) != 2 {
		t.Fatal("merged separate performances")
	}
	s.SeatGeek.Artists[0].Concerts[0].LocalTime = ""
	second := s.Ticketmaster.Artists[0].Concerts[0]
	second.InternalID = "ticketmaster:late"
	second.LocalTime = "21:00:00"
	second.Sources = []ConcertSource{{Provider: "Ticketmaster", EventID: "late", URL: "https://www.ticketmaster.com/event/late"}}
	s.Ticketmaster.Artists[0].Concerts = append(s.Ticketmaster.Artists[0].Concerts, second)
	if len(visibleReleases(s, c, testNow)) != 3 {
		t.Fatal("guessed ambiguous show time")
	}
	s, c = duplicateFixture()
	s.SeatGeek.Artists[0].Concerts[0].Venue = "Other Hall"
	if len(visibleReleases(s, c, testNow)) != 2 {
		t.Fatal("merged different venues")
	}
}
func TestDeduplicationCanonicalURLAndMissingIdentity(t *testing.T) {
	s, c := duplicateFixture()
	s.SeatGeek.Artists[0].Concerts[0].Venue = ""
	s.SeatGeek.Artists[0].Concerts[0].Sources[0].URL = "https://www.ticketmaster.com/event/show?utm_source=other"
	if len(visibleReleases(s, c, testNow)) != 1 {
		t.Fatal("did not recognize canonical event URL")
	}
	s.SeatGeek.Artists[0].Artist = Artist{ID: "other", Name: "Other"}
	s.Entries = append(s.Entries, Entry{Artist: s.SeatGeek.Artists[0].Artist})
	if len(visibleReleases(s, c, testNow)) != 2 {
		t.Fatal("merged unrelated artist identities")
	}
}
