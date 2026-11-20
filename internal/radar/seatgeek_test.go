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

func sgConfig() SeatGeekConfig {
	return SeatGeekConfig{Enabled: true, ClientID: "fixture-sg-client", ClientSecret: "fixture-sg-secret", DailyBudget: 3, Overrides: map[string]string{"ar-1": "123"}}
}
func sgEventFixture(id int64, timeTBD bool) map[string]any {
	return map[string]any{"id": id, "title": "Concert <&>", "url": "/fixture-tickets/concert/2026-12-01/123/", "type": "concert", "datetime_local": "2026-12-01T03:30:00", "datetime_utc": "2026-12-01T08:30:00", "time_tbd": timeTBD, "status": "normal", "performers": []any{map[string]int{"id": 123}}, "venue": map[string]string{"name": "Hall", "city": "London", "country": "GB"}}
}
func sgPageFixture(page, total, perPage int, events ...map[string]any) Response {
	b, _ := json.Marshal(map[string]any{"meta": map[string]int{"page": page, "total": total, "per_page": perPage}, "events": events})
	return Response{Status: 200, Body: b}
}
func TestSeatGeekDurablePagingTBDAndExport(t *testing.T) {
	sc := sgConfig()
	s := SeatGeekState{Schema: 1}
	s.Reconcile(ready().Entries, cfg(), sc.QueueConfig(), "scope", false)
	dir := t.TempDir()
	calls := 0
	fetch := func(raw string) (Response, error) {
		calls++
		saved, err := LoadSeatGeek(dir)
		if err != nil || saved.Used != calls {
			t.Fatal("missing reservation", err)
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		if u.Host != "api.seatgeek.com" || q.Get("client_id") != sc.ClientID || q.Get("client_secret") != sc.ClientSecret || q.Get("performers.id") != "123" || q.Get("page") != []string{"1", "2"}[calls-1] {
			t.Fatal("incorrect provider query")
		}
		return sgPageFixture(calls, 2, 1, sgEventFixture(int64(calls), calls == 1)), nil
	}
	persist := func(s SeatGeekState) error { return SaveSeatGeek(dir, s) }
	s.StepSeatGeek(sc, cfg(), fetch, persist, testNow)
	if len(s.Artists[0].Concerts) != 0 {
		t.Fatal("published partial snapshot")
	}
	s, _ = LoadSeatGeek(dir)
	s.StepSeatGeek(sc, cfg(), fetch, persist, testNow.Add(time.Second))
	a := s.Artists[0]
	if len(a.Concerts) != 2 || a.Concerts[0].LocalDate != "2026-12-01" || a.Concerts[0].LocalTime != "" || a.Concerts[0].StartUTC != "" || a.Concerts[1].StartUTC != "2026-12-01T08:30:00Z" {
		t.Fatalf("%+v", a)
	}
	state := ready()
	state.SeatGeek = &s
	c := cfg()
	c.SeatGeek = sc
	html, _ := Render(state, c, testNow)
	if !strings.Contains(string(html), "SeatGeek lookup status") || !strings.Contains(string(html), "Source: SeatGeek") || !strings.Contains(string(html), "https://seatgeek.com/fixture-tickets") {
		t.Fatal("missing provider presentation")
	}
	// Distinct event IDs remain separate even with the same venue and day.
	if len(Events(state, c, c.EventNamespace(), testNow, true)) != 2 {
		t.Fatal("merged separate same-provider events")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "seatgeek.json"))
	if strings.Contains(string(b), sc.ClientID) || strings.Contains(string(b), sc.ClientSecret) {
		t.Fatal("credentials persisted")
	}
}
func TestSeatGeekIdentityAndFailure(t *testing.T) {
	a := ProviderArtist{Artist: Artist{Name: "Band"}}
	if err := a.resolveSeatGeek([]byte(`{"meta":{"page":1,"per_page":200,"total":2},"performers":[{"id":1,"name":"Band"},{"id":2,"name":"Band"}]}`), testNow); err != nil || len(a.Candidates) != 2 || a.Mapping.ProviderID != "" {
		t.Fatal(a, err)
	}
	sc := sgConfig()
	s := SeatGeekState{Schema: 1}
	s.Reconcile(ready().Entries, cfg(), sc.QueueConfig(), "scope", false)
	s.StepSeatGeek(sc, cfg(), func(string) (Response, error) { return Response{}, errors.New(sc.ClientID + sc.ClientSecret) }, func(SeatGeekState) error { return nil }, testNow)
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), sc.ClientID) || strings.Contains(string(b), sc.ClientSecret) || !strings.Contains(string(b), "SeatGeek could not be reached") {
		t.Fatal("credential leak or wrong error")
	}
	sc.Overrides["ar-1"] = "not-an-id"
	if sc.Validate() == nil {
		t.Fatal("accepted nonnumeric ID")
	}
}
func TestSeatGeekUnknownDateAndInvalidPagination(t *testing.T) {
	a := ProviderArtist{Mapping: ArtistMapping{ProviderID: "123"}}
	e := sgEventFixture(1, false)
	e["date_tbd"] = true
	if err := a.seatGeekEvents(sgPageFixture(1, 1, 200, e).Body, testNow); err != nil {
		t.Fatal(err)
	}
	if a.Concerts[0].LocalDate != "" || a.Concerts[0].LocalTime != "" || a.Concerts[0].StartUTC != "" {
		t.Fatal("invented date")
	}
	for _, response := range []Response{sgPageFixture(2, 1, 200, e), sgPageFixture(1, 2, 200, e), sgPageFixture(1, 1001, 200)} {
		if a.seatGeekEvents(response.Body, testNow) == nil {
			t.Fatal("accepted inconsistent pagination")
		}
	}
}
