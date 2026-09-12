// Package recommendation contains versioned, provider-neutral contracts used
// to explain how a recommendation was discovered and qualified.
package recommendation

import (
	"encoding/json"
	"strings"
)

const (
	// CurrentSchemaVersion is the version written by the current recommendation
	// pipeline. Readers intentionally remain tolerant of older snapshots.
	CurrentSchemaVersion = 1

	EntityArtist    = "artist"
	EntityAlbum     = "album"
	EntityRecording = "recording"

	EvidenceCatalogIdentity = "catalog_identity"
	EvidenceSimilarity      = "similarity"
	EvidenceGenreProxy      = "genre_proxy"
	EvidenceLocalPreference = "local_preference"
	EvidenceLocalFit        = "local_fit"
	EvidenceInference       = "inference"

	QualificationSupported    = "supported"
	QualificationPlausible    = "plausible"
	QualificationInsufficient = "insufficient"
	QualificationContradicted = "contradicted"
)

// Reference identifies one supplied or resolved project. SuppliedText is
// retained even when resolution fails so similarly named projects are never
// silently merged.
type Reference struct {
	SuppliedText  string `json:"supplied_text"`
	Artist        string `json:"artist,omitempty"`
	Album         string `json:"album,omitempty"`
	Song          string `json:"song,omitempty"`
	CanonicalName string `json:"canonical_name,omitempty"`
	MBID          string `json:"mbid,omitempty"`
	EntityScope   string `json:"entity_scope"`
	Polarity      string `json:"polarity"`
	Resolution    string `json:"resolution"`
	Origin        string `json:"origin"`
	Notes         string `json:"notes,omitempty"`
}

// SimilarityEdge records one direct provider relationship. Match is an
// uncalibrated provider score; consumers must not sum it into certainty.
type SimilarityEdge struct {
	Source        string  `json:"source"`
	ReferenceText string  `json:"reference_text"`
	ReferenceMBID string  `json:"reference_mbid,omitempty"`
	Neighbor      string  `json:"neighbor"`
	NeighborMBID  string  `json:"neighbor_mbid,omitempty"`
	Match         float64 `json:"match,omitempty"`
}

// Evidence is scoped to the entity on which it was observed. A missing or
// empty evidence list means evidence is unavailable, not that a trait is true.
type Evidence struct {
	ID          string `json:"id,omitempty"`
	Source      string `json:"source"`
	EntityScope string `json:"entity_scope"`
	EntityID    string `json:"entity_id,omitempty"`
	Kind        string `json:"kind"`
	Claim       string `json:"claim"`
	Details     string `json:"details,omitempty"`
}

type Qualification struct {
	Status      string   `json:"status"`
	Reason      string   `json:"reason,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

// ProviderStatus describes availability without storing credentials, request
// URLs, or provider response bodies.
type ProviderStatus struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Calls    int    `json:"calls,omitempty"`
}

type StageMetric struct {
	Stage         string `json:"stage"`
	ElapsedMS     int64  `json:"elapsed_ms,omitempty"`
	ExternalCalls int    `json:"external_calls,omitempty"`
}

// Trace is safe to persist in a generation snapshot. It contains only bounded
// structured evidence and diagnostics, never secrets or unrelated history.
type Trace struct {
	SchemaVersion     int                      `json:"schema_version"`
	Legacy            bool                     `json:"legacy,omitempty"`
	References        []Reference              `json:"references,omitempty"`
	Examples          []Reference              `json:"examples,omitempty"`
	ContextSummary    []string                 `json:"context_summary,omitempty"`
	SimilarityEdges   []SimilarityEdge         `json:"similarity_edges,omitempty"`
	Providers         []ProviderStatus         `json:"providers,omitempty"`
	CandidateEvidence map[string][]Evidence    `json:"candidate_evidence,omitempty"`
	Qualifications    map[string]Qualification `json:"qualifications,omitempty"`
	SelectedIndexes   []int                    `json:"selected_indexes,omitempty"`
	FinalCandidateIDs []string                 `json:"final_candidate_ids,omitempty"`
	DegradedReason    string                   `json:"degraded_reason,omitempty"`
	ShortfallReason   string                   `json:"shortfall_reason,omitempty"`
	Stages            []StageMetric            `json:"stages,omitempty"`
}

// UnmarshalJSON provides legacy decoding for snapshots written before trace
// provenance existed. Missing provenance is explicit through Legacy and does
// not get reconstructed from candidate names or genre tags.
func (trace *Trace) UnmarshalJSON(data []byte) error {
	type plain Trace
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*trace = Trace(decoded)
	if trace.SchemaVersion == 0 {
		trace.Legacy = true
	}
	if trace.SchemaVersion == 0 {
		trace.SchemaVersion = CurrentSchemaVersion
	}
	return nil
}

func (trace Trace) MarshalJSON() ([]byte, error) {
	type plain Trace
	if trace.SchemaVersion == 0 {
		trace.SchemaVersion = CurrentSchemaVersion
	}
	return json.Marshal(plain(trace))
}

// Normalize trims contract values while retaining their meaning. It is
// deliberately conservative and does not infer missing entity/evidence data.
func (trace Trace) Normalize() Trace {
	trace.DegradedReason = strings.TrimSpace(trace.DegradedReason)
	trace.ShortfallReason = strings.TrimSpace(trace.ShortfallReason)
	for index := range trace.References {
		trace.References[index].SuppliedText = strings.TrimSpace(trace.References[index].SuppliedText)
		trace.References[index].CanonicalName = strings.TrimSpace(trace.References[index].CanonicalName)
	}
	return trace
}
