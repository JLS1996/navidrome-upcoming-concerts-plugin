package radar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGoogleCalendarRangeStatusAndLegacyRepair(t *testing.T) {
	for _, tc := range []struct{ end, status, wantEnd, wantStatus string }{
		{"2026-12-06", "CANCELLED", "2026-12-07", "cancelled"},
		{"2026-12-03", "", "2026-12-05", "confirmed"},
		{"2026-12", "", "2026-12-05", "confirmed"},
		{"", "", "2026-12-05", "confirmed"},
	} {
		t.Run(tc.end+tc.status, func(t *testing.T) {
			s := published()
			r := &s.Entries[0].Releases[0]
			r.Concert = &Concert{LocalDate: r.Date, EndDate: tc.end, Status: tc.wantStatus}
			r.Cancelled = tc.status == "CANCELLED"
			ic := icfg()
			ic.CalendarProvider = "google"
			e := Events(s, cfg(), ic.Namespace, testNow, true)[0]
			d := DeliveryState{AccessToken: "token", TokenExpiry: testNow.Add(time.Hour).Unix(), GoogleAuth: digest("\x00\x00"), CalendarTarget: digest("google\x00\x00" + ic.Namespace), Remote: map[string]*RemoteEvent{e.UID: {Present: true, ETag: "old", Hash: e.Fingerprint()}}}
			calls := 0
			send := func(req HTTPRequest) (Response, error) {
				calls++
				if req.Method != "PATCH" || req.Headers["If-Match"] != "old" {
					t.Fatalf("unprotected repair: %+v", req)
				}
				var body struct {
					Status     string
					Start, End struct{ Date string }
				}
				if err := json.Unmarshal(req.Body, &body); err != nil {
					t.Fatal(err)
				}
				if body.Status != tc.wantStatus || body.Start.Date != e.Date || body.End.Date != tc.wantEnd {
					t.Fatalf("wrong Google payload: %s", req.Body)
				}
				if !strings.Contains(EventICS(e, testNow), "DTEND;VALUE=DATE:"+strings.ReplaceAll(tc.wantEnd, "-", "")) {
					t.Fatal("ICS and Google disagree")
				}
				response, _ := json.Marshal(map[string]string{"id": googleID(e.UID), "etag": "new"})
				return Response{Status: 200, Body: response}, nil
			}
			d.Step(s, cfg(), ic, send, testNow)
			d.Step(s, cfg(), ic, send, testNow.Add(3*time.Second))
			if calls != 1 {
				t.Fatalf("repair called %d times", calls)
			}
		})
	}
}

func TestReportDetailsStayWithConcert(t *testing.T) {
	for _, sourceURL := range []string{"", "https://example.com/tour"} {
		s := published()
		s.Entries[0].Releases = nil
		for i, status := range []string{"scheduled", "postponed"} {
			id := []string{"ticketmaster:first", "ticketmaster:second"}[i]
			concert := Concert{InternalID: id, Title: status + " show", LocalDate: "2026-12-04", Status: status, LocalTime: []string{"18:00:00", "21:00:00"}[i], Sources: []ConcertSource{{Provider: "Ticketmaster", URL: sourceURL}}}
			s.Entries[0].Releases = append(s.Entries[0].Releases, concert.reportRecord())
		}
		b, err := Render(s, cfg(), testNow)
		if err != nil {
			t.Fatal(err)
		}
		articles := strings.Split(string(b), `<article class="concert"`)[1:]
		if len(articles) != 2 {
			t.Fatalf("expected two shows: %d", len(articles))
		}
		for _, article := range articles {
			article = strings.Split(article, "</article>")[0]
			if strings.Contains(article, "<h3>scheduled show</h3>") && (!strings.Contains(article, `data-status=""`) || strings.Contains(article, "21:00:00")) {
				t.Fatal("first concert inherited second concert details")
			}
			if strings.Contains(article, "<h3>postponed show</h3>") && (!strings.Contains(article, `data-status="Postponed"`) || strings.Contains(article, "18:00:00")) {
				t.Fatal("second concert inherited first concert details")
			}
		}
	}
}

func TestManualRefreshAndDisabledMusicBrainzMatching(t *testing.T) {
	c := cfg()
	artists := []Artist{{ID: "unknown", Name: "Unknown"}, {ID: "known", Name: "Known", MBID: artistID}}
	var s State
	s.Reconcile(artists, c, "scope", testNow)
	for i := range s.Entries {
		s.Entries[i].Failures = 3
		s.Entries[i].RetryAt = testNow.Add(time.Hour).Unix()
	}
	s.NextHTTP = testNow.Add(time.Minute).Unix()
	c.AutoMatch = false
	s.ApplyRefresh(false)
	s.ApplyRefresh(true)
	s.Reconcile(artists, c, "scope", testNow)
	if s.Entries[0].Phase != "unresolved" || s.Entries[1].Failures != 0 || s.Entries[1].RetryAt != 0 {
		t.Fatal("refresh did not apply the matching setting and reset artist retries")
	}
	if s.Step(func(string) (Response, error) {
		t.Fatal("manual refresh bypassed the upstream pause")
		return Response{}, nil
	}, c, testNow) {
		t.Fatal("lookup ran during cooldown")
	}
}

func TestArtistDiscoveryRequiresAListOnSuccess(t *testing.T) {
	if _, err := AccessibleLibraries(`{"subsonic-response":{"status":"ok"}}`, []int{1}); err == nil {
		t.Fatal("missing folder list was mistaken for revoked access")
	}
	if _, err := ParseArtists(`{"subsonic-response":{"status":"ok"}}`); err == nil {
		t.Fatal("missing artist list was mistaken for an empty library")
	}
	artists, err := ParseArtists(`{"subsonic-response":{"status":"ok","artists":{}}}`)
	if err != nil || len(artists) != 0 {
		t.Fatal("empty artist list was rejected")
	}
}

func TestCalendarExportWithoutListingURL(t *testing.T) {
	e := CalendarEvent{UID: "test", Date: "2026-12-06", EndDate: "2026-12", Summary: "Show"}
	ics := EventICS(e, testNow)
	if strings.Contains(ics, "\r\nURL:") || !strings.Contains(ics, "DTEND;VALUE=DATE:20261207") {
		t.Fatal("export contained an empty URI or inferred an exact end from a partial date")
	}
	e.URL = "https://example.com/venue,show;2"
	if !strings.Contains(EventICS(e, testNow), "URL:"+e.URL+"\r\n") {
		t.Fatal("calendar export changed URI delimiters")
	}
}
