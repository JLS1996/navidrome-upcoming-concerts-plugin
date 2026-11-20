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

func TestSingleExactMatchingPolicyAndCacheUpgrade(t *testing.T) {
	c := cfg()
	sc := sgConfig()
	sc.Overrides = map[string]string{}
	s := SeatGeekState{Schema: 1, Scope: "scope", Used: 123, Artists: []ProviderArtist{{Artist: ready().Entries[0].Artist, Lookup: LookupState{Outcome: "unresolved", NextRefresh: testNow.Add(7 * 24 * time.Hour).Unix()}, Candidates: []ArtistCandidate{{ID: "123", Name: "Artist"}}}}}
	s.Reconcile(ready().Entries, c, sc.QueueConfig(), "scope", false)
	if s.Used != 123 || s.Artists[0].Lookup.NextRefresh != 0 || s.Artists[0].Mapping.ProviderID != "" {
		t.Fatal("upgrade reset quota or trusted incomplete old candidates")
	}
	s.Artists[0].Lookup.NextRefresh = testNow.Add(time.Hour).Unix()
	s.Reconcile(ready().Entries, c, sc.QueueConfig(), "scope", false)
	if s.Artists[0].Lookup.NextRefresh == 0 {
		t.Fatal("upgrade repeatedly reopened retry")
	}
	s.Artists[0].Lookup.NextRefresh = 0
	raw := []byte(`{"meta":{"page":1,"per_page":200,"total":1},"performers":[{"id":123,"name":"Artist"}]}`)
	s.StepSeatGeek(sc, c, func(string) (Response, error) { return Response{Status: 200, Body: raw}, nil }, func(SeatGeekState) error { return nil }, testNow)
	if s.Artists[0].Mapping.ProviderID != "123" || s.Artists[0].Mapping.MatchMethod != "unique-exact-name" || s.Artists[0].Lookup.Outcome != "resolved" {
		t.Fatal("unique complete search did not queue concerts")
	}
	// An explicit override always wins over inferred identity.
	sc.Overrides["ar-1"] = "456"
	s.Reconcile(ready().Entries, c, sc.QueueConfig(), "scope", false)
	if s.Artists[0].Mapping.ProviderID != "456" || s.Artists[0].Mapping.MatchMethod != "manual" {
		t.Fatal("automatic matching overwrote override")
	}
}

func TestExactMatchingRequiresCompleteSearchAndOptIn(t *testing.T) {
	for _, tc := range []struct {
		auto, complete bool
		count          int
		want           bool
	}{{true, true, 1, true}, {false, true, 1, false}, {true, false, 1, false}, {true, true, 2, false}, {true, true, 0, false}} {
		a := ProviderArtist{Artist: Artist{Name: "Band"}, Lookup: LookupState{Outcome: "unresolved"}, CandidatesComplete: tc.complete}
		for i := 0; i < tc.count; i++ {
			a.Candidates = append(a.Candidates, ArtistCandidate{ID: "123", Name: "Band"})
		}
		if tc.auto {
			a.adoptUniqueCandidate()
		}
		if (a.Mapping.ProviderID != "") != tc.want {
			t.Fatalf("case %+v", tc)
		}
	}
}

func TestPaginationPinsDateWindowAcrossMidnight(t *testing.T) {
	s := tmReady()
	c := cfg()
	tc := tmConfig()
	at := time.Date(2026, 11, 20, 23, 59, 59, 0, time.UTC)
	first := ""
	calls := 0
	fetch := func(raw string) (Response, error) {
		calls++
		u, _ := url.Parse(raw)
		window := u.Query().Get("startDateTime") + "/" + u.Query().Get("endDateTime")
		if calls == 1 {
			first = window
			return tmPage(0, 2, 2, tmEvent("first", "2026-12-01", "onsale")), nil
		}
		if window != first {
			t.Fatal("pagination changed date window")
		}
		return tmPage(1, 2, 2, tmEvent("second", "2026-12-02", "onsale")), nil
	}
	s.Step(tc, c, fetch, func(TicketmasterState) error { return nil }, at)
	// Simulate persisted state and a new callback after midnight.
	b, _ := json.Marshal(s)
	json.Unmarshal(b, &s)
	s.Step(tc, c, fetch, func(TicketmasterState) error { return nil }, at.Add(2*time.Second))
	if calls != 2 || len(s.Artists[0].Concerts) != 2 {
		t.Fatal("did not complete pinned snapshot")
	}
}

func TestRepeatedPageRetainsSnapshotAndRestarts(t *testing.T) {
	a := tmReady().Artists[0]
	a.Concerts = []Concert{{InternalID: "ticketmaster:keep"}}
	a.events(tmPage(0, 2, 2, tmEvent("same", "2026-12-01", "onsale")).Body, testNow)
	if err := a.events(tmPage(1, 2, 2, tmEvent("same", "2026-12-01", "onsale")).Body, testNow); err == nil || a.Lookup.Page != 0 || a.Concerts[0].InternalID != "ticketmaster:keep" {
		t.Fatal("repeated page silently replaced good data")
	}
}

func TestOngoingConcertRangeAndRescheduleOutsideWindow(t *testing.T) {
	c := cfg()
	c.PastDays = 0
	event := Concert{LocalDate: "2026-11-18", EndDate: "2026-11-21"}
	if !Visible(event.reportRecord(), c, testNow) {
		t.Fatal("ongoing range hidden")
	}
	s, c := duplicateFixture()
	c.AheadDays = 10
	old := &s.Ticketmaster.Artists[0]
	newer := &s.SeatGeek.Artists[0]
	old.Lookup.LastSuccess = testNow.Unix()
	newer.Lookup.LastSuccess = testNow.Add(time.Hour).Unix()
	old.Concerts[0].LocalDate = "2026-11-20"
	newer.Concerts[0].LocalDate = "2027-01-01"
	newer.Concerts[0].Sources[0].URL = old.Concerts[0].Sources[0].URL
	if len(visibleReleases(s, c, testNow)) != 0 {
		t.Fatal("older in-window listing overrode newer reschedule")
	}
}

// Optional private replay. Input files stay outside version control and are never
// included in fixtures, release archives, network requests or test failure output.
func TestPrivateScanReplay(t *testing.T) {
	input := os.Getenv("CONCERT_SCAN_DIR")
	if input == "" {
		t.Skip("set CONCERT_SCAN_DIR for an offline private scan replay")
	}
	dir := t.TempDir()
	for _, pair := range [][2]string{{"3-state.json", "state.json"}, {"5-seatgeek.json", "seatgeek.json"}, {"6-skiddle.json", "skiddle.json"}, {"7-integrations.json", "integrations.json"}} {
		data, err := os.ReadFile(filepath.Join(input, pair[0]))
		if err != nil {
			t.Fatal("private fixture unavailable")
		}
		if os.WriteFile(filepath.Join(dir, pair[1]), data, 0600) != nil {
			t.Fatal("private replay write failed")
		}
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal("old state did not load")
	}
	sg, err := LoadSeatGeek(dir)
	if err != nil {
		t.Fatal("old SeatGeek state did not load")
	}
	sk, err := LoadSkiddle(dir)
	if err != nil {
		t.Fatal("old Skiddle state did not load")
	}
	c := DefaultConfig()
	c.Username = "private-replay"
	if at := strings.LastIndex(s.Scope, "|["); at >= 0 {
		var identity struct{ User string }
		if json.Unmarshal([]byte(s.Scope[:at]), &identity) == nil && identity.User != "" {
			c.Username = identity.User
		}
	}
	c.ExcludeUncredited = s.Direct.Ready
	c.DirectReady = s.Direct.Ready
	c.DirectCredits = s.Direct.Credits
	c.SeatGeek.Enabled = true
	c.Skiddle.Enabled = true
	sgUsed, skUsed := sg.Used, sk.Used
	sg.Reconcile(s.Entries, c, c.SeatGeek.QueueConfig(), s.Scope, false)
	sk.Reconcile(s.Entries, c, c.Skiddle.QueueConfig(), s.Scope, false)
	if sg.Used != sgUsed || sk.Used != skUsed {
		t.Fatal("upgrade reset actual scan budgets")
	}
	s.SeatGeek = &sg
	s.Skiddle = &sk
	// Render at the snapshot date rather than making network requests or moving events.
	now := time.Unix(s.RenderedAt, 0)
	events := Events(s, c, c.EventNamespace(), now, true)
	if raw, readErr := os.ReadFile(filepath.Join(input, "2-music-concerts.ics")); readErr == nil {
		ics := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n ", ""), "\n ", "")
		ids := map[string]bool{}
		for _, line := range strings.Split(ics, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "UID:") {
				ids[strings.TrimPrefix(line, "UID:")] = true
			}
		}
		if len(ids) != len(events) {
			t.Fatal("private replay changed calendar event count")
		}
		for _, event := range events {
			if !ids[event.UID] {
				t.Fatal("private replay changed an existing calendar identity")
			}
		}
	}
	html, err := Render(s, c, now)
	if err != nil || !strings.Contains(string(html), "concert lookups complete") {
		t.Fatal("private replay rendering failed")
	}
	if output := os.Getenv("CONCERT_REPLAY_REPORT"); output != "" {
		if os.WriteFile(output, html, 0600) != nil {
			t.Fatal("private replay report could not be written")
		}
	}
	queued := func(state TicketmasterState) int {
		n := 0
		for _, a := range state.Artists {
			if a.Lookup.NextRefresh == 0 && len(a.Candidates) == 1 {
				n++
			}
		}
		return n
	}
	t.Logf("Offline replay: library artists=%d; eligible=%d; calendar events=%d; single-candidate rechecks queued: SeatGeek=%d Skiddle=%d; request counters preserved", len(s.Entries), len(sg.Artists), len(events), queued(sg), queued(sk))
}

func TestAccessScopeMigrationPreservesOnlyAuthorizedIdentity(t *testing.T) {
	c := cfg()
	legacy := c.Fingerprint() + "|[2 1]"
	if !SameAccessScope(legacy, c.Username, []int{1, 2}) {
		t.Fatal("could not migrate legacy authorized cache")
	}
	if SameAccessScope(legacy, "another-user", []int{1, 2}) || SameAccessScope(legacy, c.Username, []int{1}) {
		t.Fatal("cache migration crossed an authorization boundary")
	}
	current := AccessScope(c.Username, []int{1, 2})
	c.Overrides = map[string]string{"ar-1": artistID}
	c.Exclusions = map[string]bool{"other": true}
	if current != AccessScope(c.Username, []int{2, 1}) {
		t.Fatal("matching preferences reset the access scope")
	}
}

func TestProviderPublicationVersionChangesAfterCommit(t *testing.T) {
	provider := tmReady()
	s := ready()
	s.Ticketmaster = &provider
	c := cfg()
	c.Ticketmaster = tmConfig()
	before := ProviderPublicationVersion(s, c)
	provider.Step(c.Ticketmaster, c, func(string) (Response, error) { return tmPage(0, 0, 0), nil }, func(TicketmasterState) error { return nil }, testNow)
	if provider.Revision != 1 || before == ProviderPublicationVersion(s, c) {
		t.Fatal("completed snapshot cannot invalidate report after restart")
	}
}

func TestConfirmingAutomaticIdentityKeepsConcertCache(t *testing.T) {
	c := cfg()
	tc := tmConfig()
	s := tmReady()
	s.Artists[0].Mapping.ManualOverride = ""
	s.Artists[0].Mapping.MatchMethod = "unique-exact-name"
	s.Artists[0].Concerts = []Concert{{InternalID: "ticketmaster:keep"}}
	s.Reconcile(ready().Entries, c, tc, "scope", false)
	if len(s.Artists[0].Concerts) != 1 || s.Artists[0].Mapping.MatchMethod != "manual" {
		t.Fatal("manual confirmation erased confirmed ID's cached concerts")
	}
}
