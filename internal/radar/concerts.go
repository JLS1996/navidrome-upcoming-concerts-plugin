package radar

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func venueSuffix(venue string) string {
	if venue == "" {
		return ""
	}
	return " · " + venue
}

// Artist event relationships are returned as one document by MusicBrainz.
// Commit only complete, validated responses so failed refreshes retain old results.
func (e *Entry) concerts(data []byte, now time.Time) error {
	var result struct {
		ID        string `json:"id"`
		Relations []struct {
			Target string `json:"target-type"`
			Event  struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Type      string `json:"type"`
				Cancelled bool   `json:"cancelled"`
				Life      struct {
					Begin string `json:"begin"`
				} `json:"life-span"`
				Relations []struct {
					Place struct {
						Name    string `json:"name"`
						Address string `json:"address"`
					} `json:"place"`
				} `json:"relations"`
			} `json:"event"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("invalid concert response: %w", err)
	}
	if result.ID != e.ResolvedMBID || result.Relations == nil {
		return fmt.Errorf("concert response identity or relationships missing")
	}
	out := []Release{}
	seen := map[string]bool{}
	for _, rel := range result.Relations {
		if rel.Target != "event" {
			continue
		}
		event := rel.Event
		if !ValidMBID(event.ID) {
			return fmt.Errorf("invalid concert event ID")
		}
		if event.Type != "Concert" || event.Cancelled || seen[event.ID] {
			continue
		}
		seen[event.ID] = true
		places := []string{}
		for _, relation := range event.Relations {
			if relation.Place.Name != "" {
				places = append(places, strings.TrimSpace(relation.Place.Name+" "+relation.Place.Address))
			}
		}
		out = append(out, Release{ID: event.ID, Title: event.Name, Date: event.Life.Begin, Type: "Concert", Venue: strings.Join(places, " · ")})
	}
	e.Releases = out
	e.Pending = nil
	e.Offset = 0
	e.Phase = "done"
	e.CheckedAt = now.UTC().Format(time.RFC3339)
	e.Problem = ""
	return nil
}
