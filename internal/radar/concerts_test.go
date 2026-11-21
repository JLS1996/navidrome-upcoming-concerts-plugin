package radar

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestConcertPipeline(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	artist := "11111111-1111-4111-8111-111111111111"
	event := "22222222-2222-4222-8222-222222222222"
	e := Entry{Artist: Artist{ID: "a", Name: "Band <&>"}, ResolvedMBID: artist, Phase: "releases"}
	body := `{"id":"` + artist + `","relations":[
        {"target-type":"url","type":"discography entry","url":{"resource":"https://discography.example.org/"}},
        {"target-type":"url","type":"official homepage","ended":true,"url":{"resource":"https://old-band.example.org/"}},
        {"target-type":"url","type":"official homepage","url":{"resource":"javascript:alert(1)"}},
        {"target-type":"url","type":"official homepage","url":{"resource":"https://band.example.org/?a=1&b=2"}},
        {"target-type":"event","event":{"id":"` + event + `","name":"Winter <show>","type":"Concert","life-span":{"begin":"2026-11-19","end":"2026-11-21"},"relations":[{"place":{"name":"Hall","address":"London","area":{"iso-3166-2-codes":["GB-LND"]}}}]}}
    ]}`
	s := State{Entries: []Entry{e}}
	if !s.Step(func(u string) (Response, error) {
		endpoint, err := url.Parse(u)
		if err != nil || !strings.HasSuffix(endpoint.Path, "artist/"+artist) || endpoint.Query().Get("inc") != "event-rels+url-rels" {
			t.Fatal(u)
		}
		return Response{Status: 200, Body: []byte(body)}, nil
	}, DefaultConfig(), now) {
		t.Fatal("no step")
	}
	if s.Entries[0].Phase != "done" || len(s.Entries[0].Releases) != 1 {
		t.Fatalf("%+v", s)
	}
	if s.Entries[0].ArtistWebsite != "https://band.example.org/?a=1&b=2" {
		t.Fatal("did not select the active official homepage")
	}
	c := DefaultConfig()
	html, err := Render(s, c, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Winter &lt;show&gt;", "Hall London", "musicbrainz.org/event/" + event, `data-timing="Ongoing"`, `data-end-date="2026-11-21"`, `data-country="GB"`, `href="https://band.example.org/?a=1&amp;b=2" rel="noreferrer">Band &lt;&amp;&gt;</a>`} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("missing %s", want)
		}
	}
	events := Events(s, c, c.EventNamespace(), now, true)
	if len(events) != 1 {
		t.Fatal(events)
	}
	ics := strings.ReplaceAll(string(CalendarICS(events, now)), "\r\n ", "")
	for _, want := range []string{"DTSTART;VALUE=DATE:20261119", "DTEND;VALUE=DATE:20261122", "Hall London", "@concert-radar"} {
		if !strings.Contains(ics, want) {
			t.Fatal(want)
		}
	}
	old := s.Entries[0].Releases[0]
	if err := s.Entries[0].concerts([]byte(`{"id":"wrong"}`), now); err == nil {
		t.Fatal("accepted malformed response")
	}
	if s.Entries[0].Releases[0].ID != old.ID || s.Entries[0].ArtistWebsite == "" {
		t.Fatal("lost previous results")
	}
}

func TestConcertFiltering(t *testing.T) {
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	c := DefaultConfig()
	for _, tc := range []struct {
		date            string
		cancelled, want bool
	}{{"2026-11-19", false, false}, {"2026-11-20", false, true}, {"2026-12", false, true}, {"2026-12-01", true, false}, {"bogus", false, false}, {"2028-01-01", false, false}} {
		if got := Visible(Release{Type: "Concert", Date: tc.date, Cancelled: tc.cancelled}, c, now); got != tc.want {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
}
