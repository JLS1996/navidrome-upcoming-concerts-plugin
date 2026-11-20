package radar

import "time"

// Name-only identity is a configurable heuristic, never a proof of identity.
// Ambiguous/truncated searches and explicit overrides always take precedence.
func (a *ProviderArtist) adoptUniqueCandidate() {
	if a.Mapping.ProviderID != "" || a.Mapping.ManualOverride != "" || a.Lookup.Outcome != "unresolved" || !a.CandidatesComplete || len(a.Candidates) != 1 {
		return
	}
	candidate := a.Candidates[0]
	if !validProviderID(candidate.ID) || normalize(candidate.Name) != normalize(a.Artist.Name) {
		return
	}
	a.Mapping.ProviderID = candidate.ID
	a.Mapping.MatchMethod = "unique-exact-name"
	a.Lookup.Outcome = "resolved"
	a.Lookup.NextRefresh = 0
}

// A paginated snapshot must use one date window even if fetching crosses midnight.
func providerQueryDay(a *ProviderArtist, now time.Time) time.Time {
	if day, err := time.Parse("2006-01-02", a.QueryDay); err == nil {
		return day
	}
	return now.UTC().Truncate(24 * time.Hour)
}
