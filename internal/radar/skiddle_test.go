package radar

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func skConfig() SkiddleConfig {
	return SkiddleConfig{Enabled: true, APIKey: "fixture-sk-key", DailyBudget: 2, Overrides: map[string]string{"ar-1": "456"}}
}
func skEventFixture() map[string]any {
	return map[string]any{"id": "789", "eventname": "Fixture live show", "date": "2026-12-01", "enddate": "2026-12-02", "link": "https://www.skiddle.com/whats-on/fixture/789/?utm_source=skiddle", "cancelled": "0", "artists": []any{map[string]string{"artistid": "456"}}, "venue": map[string]string{"name": "Hall", "town": "London", "country": "GB"}, "openingtimes": map[string]string{"doorsopen": "19:00"}}
}
func skResponse(events ...map[string]any) Response {
	b, _ := json.Marshal(map[string]any{"error": 0, "totalcount": len(events), "results": events})
	return Response{Status: 200, Body: b}
}
func TestSkiddleReservationAttributionAndDoors(t *testing.T) {
	sc := skConfig()
	s := SkiddleState{Schema: 1}
	s.Reconcile(ready().Entries, cfg(), sc.QueueConfig(), "scope", false)
	dir := t.TempDir()
	fetch := func(raw string) (Response, error) {
		saved, err := LoadSkiddle(dir)
		if err != nil || saved.Used != 1 {
			t.Fatal(err, saved)
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		if u.Host != "www.skiddle.com" || q.Get("a") != "456" || q.Get("description") != "1" || q.Get("offset") != "0" {
			t.Fatal("incorrect query")
		}
		return skResponse(skEventFixture()), nil
	}
	s.StepSkiddle(sc, cfg(), fetch, func(s SkiddleState) error { return SaveSkiddle(dir, s) }, testNow)
	if len(s.Artists[0].Concerts) != 1 {
		t.Fatal(s)
	}
	concert := s.Artists[0].Concerts[0]
	if concert.DoorsTime != "19:00" || concert.LocalTime != "" || concert.EndDate != "2026-12-02" || concert.Country != "GB" {
		t.Fatal("invented start time or lost range", concert)
	}
	if concert.Sources[0].URL != skEventFixture()["link"] {
		t.Fatal("changed source link")
	}
	state := ready()
	state.Skiddle = &s
	c := cfg()
	c.Skiddle = sc
	html, _ := Render(state, c, testNow)
	for _, want := range []string{"Skiddle listing", "Skiddle logo", "Skiddle lookup status", "Doors open: 19:00"} {
		if !strings.Contains(string(html), want) {
			t.Fatal("missing", want)
		}
	}
	events := Events(state, c, c.EventNamespace(), testNow, true)
	if len(events) != 1 || !strings.Contains(events[0].Description, SkiddleLogoURL) {
		t.Fatal(events)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "skiddle.json"))
	if strings.Contains(string(b), sc.APIKey) {
		t.Fatal("stored API key")
	}
}
func TestSkiddleStopsClientErrorsAndExhaustedFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 503} {
		sc := skConfig()
		sc.DailyBudget = 10
		s := SkiddleState{Schema: 1}
		s.Reconcile(ready().Entries, cfg(), sc.QueueConfig(), "scope", false)
		now := testNow
		calls := 0
		fetch := func(string) (Response, error) { calls++; return Response{Status: status}, nil }
		persist := func(SkiddleState) error { return nil }
		limit := 1
		if status == 503 {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			s.StepSkiddle(sc, cfg(), fetch, persist, now)
			now = time.Unix(max(s.PauseUntil, s.Artists[0].Lookup.NextRefresh), 0)
		}
		if !s.Blocked || calls != limit {
			t.Fatal(status, s)
		}
		s.StepSkiddle(sc, cfg(), fetch, persist, now.Add(48*time.Hour))
		if calls != limit {
			t.Fatal("retried blocked provider")
		}
		s.Reconcile(ready().Entries, cfg(), sc.QueueConfig(), "scope", true)
		if s.Blocked {
			t.Fatal("manual refresh did not unblock")
		}
	}
}
func TestSkiddleIdentityAndErrors(t *testing.T) {
	a := ProviderArtist{Artist: Artist{Name: "Band"}}
	if err := a.resolveSkiddle([]byte(`{"error":"0","totalcount":"1","results":[{"id":"456","name":"Band"}]}`), testNow); err != nil || len(a.Candidates) != 1 || a.Mapping.ProviderID != "" {
		t.Fatal(a, err)
	}
	a.Mapping.ProviderID = "456"
	if a.skiddleEvents([]byte(`{"error":1,"totalcount":0,"results":[]}`), testNow) == nil {
		t.Fatal("provider error treated as empty results")
	}
	e := skEventFixture()
	delete(e, "artists")
	if a.skiddleEvents(skResponse(e).Body, testNow) == nil {
		t.Fatal("missing artist credits treated as authoritative")
	}
}
func TestDeduplicationThreeSourcesPreservesKnownStatus(t *testing.T) {
	s, c := duplicateFixture()
	c.Skiddle = skConfig()
	base := s.Ticketmaster.Artists[0].Concerts[0]
	base.InternalID = "skiddle:789"
	base.LocalTime = ""
	base.Status = "unknown"
	base.Sources = []ConcertSource{{Provider: "Skiddle", EventID: "789", URL: "https://www.skiddle.com/whats-on/fixture/789/"}}
	s.Skiddle = &SkiddleState{Schema: 1, Artists: []ProviderArtist{{Artist: s.Entries[0].Artist, Lookup: LookupState{LastSuccess: testNow.Unix()}, Concerts: []Concert{base}}}}
	rows := visibleReleases(s, c, testNow)
	if len(rows) != 1 || len(rows[0].Concert.Sources) != 3 || rows[0].Concert.Status == "unknown" {
		t.Fatal(rows)
	}
	c.DeduplicateConcerts = false
	if len(visibleReleases(s, c, testNow)) != 3 {
		t.Fatal("dedup switch ignored")
	}
}
