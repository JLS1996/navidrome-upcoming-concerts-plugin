package radar

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tmConfig() TicketmasterConfig {
	return TicketmasterConfig{Enabled: true, APIKey: "fixture-key-never-real", DailyBudget: 4, Overrides: map[string]string{"ar-1": "band_1"}}
}
func tmReady() TicketmasterState {
	s := TicketmasterState{Schema: 1}
	s.Reconcile(ready().Entries, cfg(), tmConfig(), "scope", false)
	return s
}
func tmEvent(id, date, status string) map[string]any {
	return map[string]any{"id": id, "name": "Show <&>", "url": "https://www.ticketmaster.com/event/" + id, "dates": map[string]any{"timezone": "Europe/London", "start": map[string]any{"localDate": date, "localTime": "19:30:00"}, "status": map[string]any{"code": status}}, "_embedded": map[string]any{"attractions": []any{map[string]string{"id": "band_1"}}, "venues": []any{map[string]any{"name": "Hall <&>", "city": map[string]string{"name": "London"}, "country": map[string]string{"countryCode": "GB"}}}}}
}
func tmPage(page, total, pages int, events ...map[string]any) Response {
	b, _ := json.Marshal(map[string]any{"page": map[string]int{"number": page, "totalElements": total, "totalPages": pages}, "_embedded": map[string]any{"events": events}})
	return Response{Status: 200, Body: b}
}
func TestTicketmasterDurablePaginationAndReports(t *testing.T) {
	s := tmReady()
	tc := tmConfig()
	dir := t.TempDir()
	now := testNow
	calls := 0
	persist := func(s TicketmasterState) error { return SaveTicketmaster(dir, s) }
	fetch := func(raw string) (Response, error) {
		calls++
		disk, err := LoadTicketmaster(dir)
		if err != nil || disk.Used != calls {
			t.Fatal("HTTP preceded durable reservation", disk, err)
		}
		u, _ := url.Parse(raw)
		if u.Host != "app.ticketmaster.com" || u.Query().Get("attractionId") != "band_1" || u.Query().Get("size") != "200" || u.Query().Get("classificationName") != "music" {
			t.Fatal(raw)
		}
		if calls == 1 {
			return tmPage(0, 2, 2, tmEvent("show_1", "2026-12-01", "onsale")), nil
		}
		if u.Query().Get("page") != "1" {
			t.Fatal("lost pagination")
		}
		return tmPage(1, 2, 2, tmEvent("show_2", "2026-12-02", "canceled")), nil
	}
	if _, err := s.Step(tc, cfg(), fetch, persist, now); err != nil {
		t.Fatal(err)
	}
	if len(s.Artists[0].Concerts) != 0 || s.Artists[0].Lookup.Page != 1 {
		t.Fatal("published partial page")
	}
	var err error
	s, err = LoadTicketmaster(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Step(tc, cfg(), fetch, persist, now.Add(time.Second))
	if len(s.Artists[0].Concerts) != 2 || s.Artists[0].Lookup.Outcome != "found" {
		t.Fatalf("%+v", s)
	}
	state := ready()
	state.Ticketmaster = &s
	c := cfg()
	c.Ticketmaster = tc
	html, err := Render(state, c, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Show &lt;&amp;&gt;", "Hall &lt;&amp;&gt;, London, GB", `aria-label="Ticketmaster listing"`, `data-status="Cancelled"`, "2 / 4 locally reserved"} {
		if !strings.Contains(string(html), want) {
			t.Fatal("missing", want)
		}
	}
	events := Events(state, c, c.EventNamespace(), now, true)
	if len(events) != 2 {
		t.Fatal(events)
	}
	ics := string(CalendarICS(events, now))
	if !strings.Contains(ics, "STATUS:CANCELLED") || !strings.Contains(ics, "Local time") {
		t.Fatal(ics)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ticketmaster.json"))
	if strings.Contains(string(b), tc.APIKey) {
		t.Fatal("persisted API key")
	}
}
func TestTicketmasterBudgetCannotResetOnRestartScopeOrRefresh(t *testing.T) {
	s := tmReady()
	tc := tmConfig()
	tc.DailyBudget = 1
	dir := t.TempDir()
	calls := 0
	fetch := func(string) (Response, error) { calls++; return Response{Status: 503}, nil }
	save := func(s TicketmasterState) error { return SaveTicketmaster(dir, s) }
	s.Step(tc, cfg(), fetch, save, testNow)
	s, _ = LoadTicketmaster(dir)
	s.Reconcile(ready().Entries, cfg(), tc, "other-scope", true)
	s.Step(tc, cfg(), fetch, save, testNow.Add(time.Hour))
	if calls != 1 || s.Used != 1 {
		t.Fatal("budget reset")
	}
	s.Step(tc, cfg(), fetch, save, testNow.Add(24*time.Hour))
	if calls != 2 || s.Used != 1 {
		t.Fatal("budget failed to renew", calls, s)
	}
	// Clock moving backward must not reopen a previous day's budget.
	s.Step(tc, cfg(), fetch, save, testNow)
	if calls != 2 {
		t.Fatal("clock rollback reopened budget")
	}
}
func TestTicketmasterSaveFailurePreventsRequestAndErrorsRedactKey(t *testing.T) {
	s := tmReady()
	calls := 0
	tc := tmConfig()
	fetch := func(string) (Response, error) {
		calls++
		return Response{}, errors.New("sensitive URL?apikey=" + tc.APIKey)
	}
	_, err := s.Step(tc, cfg(), fetch, func(TicketmasterState) error { return errors.New("disk full") }, testNow)
	if err == nil || calls != 0 {
		t.Fatal("dispatch despite save failure")
	}
	s = tmReady()
	s.Step(tc, cfg(), fetch, func(TicketmasterState) error { return nil }, testNow)
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), tc.APIKey) {
		t.Fatal("secret in error")
	}
	if s.Artists[0].Lookup.Outcome != "failed" {
		t.Fatal(s)
	}
}
func TestTicketmasterFailureRetainsCacheAndHonorsRetryAfter(t *testing.T) {
	s := tmReady()
	a := &s.Artists[0]
	a.Concerts = []Concert{{InternalID: "ticketmaster:keep"}}
	tc := tmConfig()
	s.Step(tc, cfg(), func(string) (Response, error) {
		return Response{Status: 429, Headers: map[string]string{"Retry-After": "120"}}, nil
	}, func(TicketmasterState) error { return nil }, testNow)
	if s.PauseUntil != testNow.Unix()+120 || len(a.Concerts) != 1 || a.Lookup.Outcome != "failed" {
		t.Fatal(s)
	}
	called := false
	s.Step(tc, cfg(), func(string) (Response, error) { called = true; return Response{}, nil }, func(TicketmasterState) error { return nil }, testNow.Add(time.Minute))
	if called {
		t.Fatal("ignored cooldown")
	}
}
func TestTicketmasterIdentityNeedsEvidence(t *testing.T) {
	a := ProviderArtist{Artist: Artist{ID: "a", Name: "Same Name"}, Mapping: ArtistMapping{MusicBrainzID: artistID}}
	response := `{"page":{"totalElements":2},"_embedded":{"attractions":[{"id":"one","name":"Same Name"},{"id":"two","name":"Same Name"}]}}`
	if err := a.resolve([]byte(response), testNow); err != nil {
		t.Fatal(err)
	}
	if a.Mapping.ProviderID != "" || a.Lookup.Outcome != "unresolved" || len(a.Candidates) != 2 {
		t.Fatal(a)
	}
	response = `{"page":{"totalElements":1},"_embedded":{"attractions":[{"id":"one","name":"Same Name","externalLinks":{"musicbrainz":[{"url":"https://musicbrainz.org/artist/` + artistID + `"}]}}]}}`
	if err := a.resolve([]byte(response), testNow); err != nil || a.Mapping.ProviderID != "one" {
		t.Fatal(a, err)
	}
}
func TestTicketmasterNormalizationAndEmptyRefresh(t *testing.T) {
	a := tmReady().Artists[0]
	e := tmEvent("unknown", "", "postponed")
	e["url"] = "javascript:alert(1)"
	delete(e, "_embedded")
	// Listings without an explicit attraction credit must be ignored.
	if err := a.events(tmPage(0, 1, 1, e).Body, testNow); err != nil || len(a.Concerts) != 0 {
		t.Fatal(a, err)
	}
	e = tmEvent("unknown", "", "postponed")
	e["url"] = "javascript:alert(1)"
	embedded := e["_embedded"].(map[string]any)
	delete(embedded, "venues")
	if err := a.events(tmPage(0, 1, 1, e).Body, testNow); err != nil {
		t.Fatal(err)
	}
	concert := a.Concerts[0]
	if concert.LocalDate != "" || concert.Venue != "" || len(concert.TicketURLs) != 0 || concert.DatePrecision != "unknown" {
		t.Fatal(concert)
	}
	c := cfg()
	if !Visible(concert.reportRecord(), c, testNow) {
		t.Fatal("undated concert hidden")
	}
	if err := a.events(tmPage(0, 0, 0).Body, testNow); err != nil || a.Lookup.Outcome != "empty" || a.Lookup.NextRefresh != testNow.Add(72*time.Hour).Unix() {
		t.Fatal(a, err)
	}
}
func TestTicketmasterTruncationAndChangedPagesKeepOldSnapshot(t *testing.T) {
	a := tmReady().Artists[0]
	a.Concerts = []Concert{{InternalID: "ticketmaster:keep"}}
	for _, r := range []Response{tmPage(0, 1001, 6), tmPage(0, 2, 1, tmEvent("only", "2026-12-01", "onsale"))} {
		if err := a.events(r.Body, testNow); err == nil || len(a.Concerts) != 1 {
			t.Fatal("incomplete response replaced cache")
		}
	}
	if err := a.events(tmPage(0, 2, 2, tmEvent("first", "2026-12-01", "onsale")).Body, testNow); err != nil {
		t.Fatal(err)
	}
	if err := a.events(tmPage(1, 3, 2, tmEvent("second", "2026-12-01", "onsale")).Body, testNow); err == nil || a.Lookup.Page != 0 || a.Concerts[0].InternalID != "ticketmaster:keep" {
		t.Fatal(a)
	}
}
func TestTicketmasterSharedEventsPreserveSeparatePerformancesAndScope(t *testing.T) {
	s := tmReady()
	a := &s.Artists[0]
	e := tmEvent("first", "2026-12-01", "onsale")
	e2 := tmEvent("second", "2026-12-01", "onsale")
	if err := a.events(tmPage(0, 2, 1, e, e2).Body, testNow); err != nil {
		t.Fatal(err)
	}
	s.Artists = append(s.Artists, s.Artists[0])
	s.Artists[1].Artist = Artist{ID: "other", Name: "Other band"}
	state := ready()
	state.Entries = append(state.Entries, Entry{Artist: s.Artists[1].Artist})
	state.Ticketmaster = &s
	c := cfg()
	c.Ticketmaster = tmConfig()
	events := Events(state, c, c.EventNamespace(), testNow, true)
	if len(events) != 2 || !strings.Contains(events[0].Summary, "Other band") {
		t.Fatal(events)
	}
	state.Entries = nil
	if len(Events(state, c, c.EventNamespace(), testNow, true)) != 0 {
		t.Fatal("scope leak")
	}
}

func TestTicketmasterDateRangeAndRefreshBarrier(t *testing.T) {
	a := tmReady().Artists[0]
	event := tmEvent("festival", "2026-12-01", "rescheduled")
	event["dates"].(map[string]any)["end"] = map[string]string{"localDate": "2026-12-03"}
	if err := a.events(tmPage(0, 1, 1, event).Body, testNow); err != nil {
		t.Fatal(err)
	}
	state := ready()
	state.Entries[0].Phase = "done"
	state.Ticketmaster = &TicketmasterState{Schema: 1, Artists: []ProviderArtist{a}}
	c := cfg()
	c.Ticketmaster = tmConfig()
	events := Events(state, c, c.EventNamespace(), testNow, true)
	if len(events) != 1 || !strings.Contains(EventICS(events[0], testNow), "DTEND;VALUE=DATE:20261204") {
		t.Fatal(events)
	}
	if !FullCycle(state, c) {
		t.Fatal("completed provider blocked cycle")
	}
	state.Ticketmaster.Artists[0].Lookup.Outcome = "fetching"
	if FullCycle(state, c) {
		t.Fatal("partial provider allowed destructive calendar reconciliation")
	}
}

func TestProviderIDValidation(t *testing.T) {
	for _, value := range []string{"", "bad/id", "with space", "é", strings.Repeat("a", 129)} {
		if validProviderID(value) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"K8vZ917Gku7", "Z7r9jZ1Ad", "abc_-123", strings.Repeat("a", 128)} {
		if !validProviderID(value) {
			t.Fatal(value)
		}
	}
}
