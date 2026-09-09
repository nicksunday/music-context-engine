package evaluation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStarterCorpusEvaluatesDeterministically(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "recommendation-fit", "starter.jsonl")
	cases, corpusHash, err := LoadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	report := Evaluate(cases, corpusHash)
	if report.Summary.Passed != 4 || report.Summary.Ineligible != 1 || report.Summary.Failed != 0 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	repeat := Evaluate(cases, corpusHash)
	if report.CorpusHash != repeat.CorpusHash || len(report.Cases) != len(repeat.Cases) {
		t.Fatal("evaluation was not repeatable")
	}
}

func TestCorpusValidationRejectsMalformedAndUnsupportedCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte(`{"schema_version":2,"case_id":"bad"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCorpus(path); err == nil || !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCorpus(path); err == nil {
		t.Fatal("malformed corpus accepted")
	}
}

func TestEvaluationReportsFailureAndBaselineNonComparable(t *testing.T) {
	item := Case{SchemaVersion: 1, CaseID: "case", Mode: "album", Stage: "ranking", Request: Request{Message: "test"}, Candidates: []Candidate{{ID: "a", Rank: 2, Eligible: true}, {ID: "b", Rank: 1, Eligible: true}}, Assertions: []Assertion{{Type: "rank_before", First: "a", Second: "b"}}}
	report := Evaluate([]Case{item}, "hash")
	if report.Summary.Failed != 1 || report.Cases[0].Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
	old := report
	item.ContentHash = "changed"
	current := Evaluate([]Case{item}, "other")
	comparison := Compare(current, old)
	if len(comparison.NonComparable) != 1 || comparison.NonComparable[0] != "case" {
		t.Fatalf("comparison = %#v", comparison)
	}
}

func TestEvaluationMarksZeroEligibleCorpusAsIneligible(t *testing.T) {
	item := Case{SchemaVersion: 1, CaseID: "ineligible", Mode: "album", Stage: "selection", Request: Request{Message: "subjective match"}}
	report := Evaluate([]Case{item}, "hash")
	if report.Summary.Eligible != 0 || report.Summary.Ineligible != 1 || report.Summary.Passed != 0 {
		t.Fatalf("summary = %#v", report.Summary)
	}
}

func TestUnsupportedAssertionFailsExplicitly(t *testing.T) {
	item := Case{SchemaVersion: 1, CaseID: "unsupported", Mode: "album", Stage: "selection", Request: Request{Message: "test"}, Assertions: []Assertion{{Type: "run_arbitrary_code"}}}
	report := Evaluate([]Case{item}, "hash")
	if report.Summary.Failed != 1 || report.Cases[0].Assertions[0].Passed {
		t.Fatalf("report = %#v", report)
	}
}
