package web

import "testing"

func TestReplayOfflineCandidatePolicyIsSeededAndReusesProductionRules(t *testing.T) {
	input := []OfflineCandidate{
		{ID: "safe", Artist: "Safe Artist", Album: "Safe Album", GenreTags: []string{"progressive metal"}, Eligible: true},
		{ID: "avoided", Artist: "Avoided Artist", Album: "Avoided Album", GenreTags: []string{"death metal"}, Eligible: true},
		{ID: "ineligible", Artist: "Ineligible Artist", Album: "Nope", GenreTags: []string{"progressive metal"}, Eligible: false},
	}
	request := OfflinePolicyRequest{Message: "progressive metal", Avoid: "death metal", Mode: "album"}
	first := ReplayOfflineCandidatePolicy(request, input, 7)
	second := ReplayOfflineCandidatePolicy(request, input, 7)
	if len(first) != 1 || first[0].ID != "safe" || first[0].Rank != 1 {
		t.Fatalf("replayed candidates = %#v", first)
	}
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("replay was not deterministic: first=%#v second=%#v", first, second)
	}
}
