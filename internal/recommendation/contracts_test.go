package recommendation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTraceDecodesLegacySnapshotWithoutFabricatingProvenance(t *testing.T) {
	var trace Trace
	if err := json.Unmarshal([]byte(`{"selected_indexes":[1],"final_candidate_ids":["candidate-1"]}`), &trace); err != nil {
		t.Fatalf("decode legacy trace: %v", err)
	}
	if !trace.Legacy || trace.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("expected legacy trace version marker, got %+v", trace)
	}
	if len(trace.SimilarityEdges) != 0 || len(trace.CandidateEvidence) != 0 {
		t.Fatalf("legacy trace fabricated provenance: %+v", trace)
	}
}

func TestTraceRoundTripsVersionedContracts(t *testing.T) {
	original := Trace{
		SchemaVersion:     CurrentSchemaVersion,
		References:        []Reference{{SuppliedText: "Symphony X", EntityScope: EntityArtist, Polarity: "positive", Resolution: "resolved"}},
		SimilarityEdges:   []SimilarityEdge{{Source: "last.fm", ReferenceText: "Symphony X", Neighbor: "Dream Theater", Match: 0.81}},
		CandidateEvidence: map[string][]Evidence{"candidate-1": {{Source: "musicbrainz", EntityScope: EntityRecording, Kind: EvidenceCatalogIdentity, Claim: "verified recording"}}},
		Qualifications:    map[string]Qualification{"candidate-1": {Status: QualificationPlausible, EvidenceIDs: []string{"e1"}}},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("encode trace: %v", err)
	}
	if strings.Contains(string(raw), "api_key") {
		t.Fatalf("trace contains credential-like field: %s", raw)
	}
	var decoded Trace
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if decoded.SchemaVersion != CurrentSchemaVersion || decoded.SimilarityEdges[0].Neighbor != "Dream Theater" || decoded.Qualifications["candidate-1"].Status != QualificationPlausible {
		t.Fatalf("trace did not round-trip: %+v", decoded)
	}
}
