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
			if strings.Contains(article, "scheduled show") && (!strings.Contains(article, "Status: scheduled") || strings.Contains(article, "21:00:00")) {
				t.Fatal("first concert inherited second concert details")
			}
			if strings.Contains(article, "postponed show") && (!strings.Contains(article, "Status: postponed") || strings.Contains(article, "18:00:00")) {
				t.Fatal("second concert inherited first concert details")
			}
		}
	}
}
