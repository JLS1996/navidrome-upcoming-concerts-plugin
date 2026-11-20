package radar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConcertSelectorsAndUTCWindow(t *testing.T) {
	c := cfg()
	b, _ := Render(published(), c, testNow)
	if !strings.Contains(string(b), `<option value="Concert">`) || strings.Contains(string(b), `<option value="Album">`) {
		t.Fatal("dashboard exposes release selectors")
	}
	c.PastDays = 0
	if Visible(Release{Type: "Concert", Date: "2026-11-19"}, c, time.Date(2026, 11, 19, 23, 30, 0, 0, time.FixedZone("local", -5*3600))) {
		t.Fatal("window used local date instead of UTC")
	}
	if Visible(Release{Type: "Album", Date: "2026-12-01"}, c, testNow) {
		t.Fatal("accepted release as a concert")
	}
}

func TestSharedReleaseHasConsistentCreditsAndNoDuplicates(t *testing.T) {
	s := published()
	other := s.Entries[0]
	other.Artist.ID = "other"
	other.Artist.Name = "Another artist"
	s.Entries = append(s.Entries, other)
	events := Events(s, cfg(), cfg().EventNamespace(), testNow, true)
	if len(events) != 1 || !strings.Contains(events[0].Summary, "Another artist / Artist") {
		t.Fatal("shared event credits", events)
	}
	b, _ := Render(s, cfg(), testNow)
	if strings.Count(string(b), `<article class="concert"`) != 1 || !strings.Contains(string(b), "Another artist / Artist") {
		t.Fatal("report/calendar deduplication differs")
	}
	// A newer source snapshot wins, even if it moves the date outside the window.
	s.Entries[1].Releases = append([]Release(nil), other.Releases...)
	s.Entries[1].Releases[0].Date = "2028-01-01"
	s.Entries[1].CheckedAt = "2026-11-20T00:00:00Z"
	if len(Events(s, cfg(), cfg().EventNamespace(), testNow, true)) != 0 {
		t.Fatal("old duplicate overrode newer metadata")
	}
}

func TestConcertRejectsInvalidResponseAndClearsRecoveredWarning(t *testing.T) {
	s := ready()
	s.Entries[0].Releases = []Release{{ID: releaseID, Title: "Keep"}}
	s.Step(func(string) (Response, error) {
		return Response{Status: 200, Body: []byte(`{"id":"wrong","relations":[]}`)}, nil
	}, cfg(), testNow)
	if s.Entries[0].Problem == "" || s.Entries[0].Releases[0].Title != "Keep" {
		t.Fatal(s)
	}
	at := time.Unix(s.Entries[0].RetryAt, 0)
	s.Step(func(string) (Response, error) { return page(nil, 0, 0), nil }, cfg(), at)
	if s.Entries[0].Problem != "" || s.Entries[0].Phase != "done" {
		t.Fatal(s)
	}
}

func TestCalendarUpdateRecoversWhenRemoteEventWasDeleted(t *testing.T) {
	for _, provider := range []string{"caldav", "google"} {
		t.Run(provider, func(t *testing.T) {
			s := published()
			c := cfg()
			ic := icfg()
			ic.CalendarProvider = provider
			ic.CalDAVURL = "https://example.com/calendar/"
			ic.GoogleCalendarID = "calendar@example.com"
			uid := Events(s, c, ic.Namespace, testNow, true)[0].UID
			desired := Events(s, c, ic.Namespace, testNow, true)[0]
			d := DeliveryState{Remote: map[string]*RemoteEvent{uid: {Present: true, Phase: "write", ETag: "old"}}, AccessToken: "fixture", TokenExpiry: testNow.Add(time.Hour).Unix()}
			d.init()
			sendMissing := func(HTTPRequest) (Response, error) { return Response{Status: 404}, nil }
			if provider == "caldav" {
				d.caldavOp(uid, desired, true, d.Remote[uid], ic, sendMissing, testNow)
			} else {
				d.googleOp(uid, desired, true, d.Remote[uid], ic, sendMissing, testNow)
			}
			if d.Remote[uid].Phase != "get" {
				t.Fatal("missing remote event did not trigger re-read")
			}
			// Re-read confirms absence, allowing a protected recreation.
			if provider == "caldav" {
				d.caldavOp(uid, desired, true, d.Remote[uid], ic, sendMissing, testNow)
			} else {
				d.googleOp(uid, desired, true, d.Remote[uid], ic, func(HTTPRequest) (Response, error) { return Response{Status: 410}, nil }, testNow)
			}
			if d.Remote[uid].Present || d.Remote[uid].Phase != "write" {
				t.Fatal("missing event was not prepared for recreation")
			}
			confirm := func(r HTTPRequest) (Response, error) {
				if provider == "caldav" {
					if r.Method != "PUT" || r.Headers["If-None-Match"] != "*" {
						t.Fatal("unprotected CalDAV recreation")
					}
					return Response{Status: 201, Headers: map[string]string{"ETag": "new"}}, nil
				}
				if r.Method != "POST" {
					t.Fatal("Google recreation was not an insert")
				}
				b, _ := json.Marshal(map[string]string{"etag": "new", "id": googleID(uid)})
				return Response{Status: 200, Body: b}, nil
			}
			if provider == "caldav" {
				d.caldavOp(uid, desired, true, d.Remote[uid], ic, confirm, testNow)
			} else {
				d.googleOp(uid, desired, true, d.Remote[uid], ic, confirm, testNow)
			}
			if !d.Remote[uid].Present || d.Remote[uid].Hash != desired.Fingerprint() {
				t.Fatal("recreated event not acknowledged")
			}
		})
	}
}

func TestInvalidPersistedRetryMetadataIsRejected(t *testing.T) {
	for _, change := range []func(*State){
		func(s *State) { s.MusicBrainzFailures = -1 },
		func(s *State) { s.MusicBrainzFailures = 8 },
		func(s *State) { s.NextHTTP = -1 },
		func(s *State) { s.Entries[0].Failures = -1 },
		func(s *State) { s.Entries[0].Failures = 9 },
		func(s *State) { s.Entries[0].RetryAt = -1 },
	} {
		s := ready()
		s.Schema = 1
		change(&s)
		b, _ := json.Marshal(s)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "state.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("invalid retry metadata accepted")
		}
	}
}

func TestRefreshSwitchBothTransitionsSurviveRestarts(t *testing.T) {
	s := ready()
	s.Entries[0].Phase = "done"
	s.Entries[0].Releases = []Release{{ID: releaseID, Title: "Keep"}}
	if s.ApplyRefresh(false) || s.RefreshPending {
		t.Fatal("initial value must establish a baseline")
	}
	for _, value := range []bool{true, false} {
		if !s.ApplyRefresh(value) || !s.RefreshPending {
			t.Fatal("saved toggle did not request a refresh", value)
		}
		b, _ := json.Marshal(s)
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		s.Reconcile([]Artist{s.Entries[0].Artist}, cfg(), s.Scope, testNow)
		if s.RefreshPending || s.Entries[0].Releases[0].Title != "Keep" {
			t.Fatal("refresh lost the prior snapshot")
		}
		if s.ApplyRefresh(value) {
			t.Fatal("unchanged switch repeated a refresh")
		}
	}
}
