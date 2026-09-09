package evaluation

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	SchemaVersion    = 1
	EvaluatorVersion = "1"
)

type Case struct {
	SchemaVersion int            `json:"schema_version"`
	CaseID        string         `json:"case_id"`
	Mode          string         `json:"mode"`
	Stage         string         `json:"stage"`
	Request       Request        `json:"request"`
	Candidates    []Candidate    `json:"candidates,omitempty"`
	Assertions    []Assertion    `json:"assertions"`
	Plan          map[string]any `json:"plan,omitempty"`
	ContentHash   string         `json:"content_hash,omitempty"`
}

type Request struct {
	Message string `json:"message"`
	Mood    string `json:"mood,omitempty"`
	Avoid   string `json:"avoid,omitempty"`
}

type Candidate struct {
	ID        string   `json:"id"`
	Artist    string   `json:"artist"`
	Album     string   `json:"album"`
	Song      string   `json:"song,omitempty"`
	GenreTags []string `json:"genre_tags,omitempty"`
	Rank      int      `json:"rank"`
	Eligible  bool     `json:"eligible"`
}

// CandidatePolicy is an optional deterministic production-policy adapter.
// It is deliberately expressed in evaluator types so the evaluator remains
// independent of web, database, and network packages.
type CandidatePolicy func(Request, string, []Candidate, int64) []Candidate

type Assertion struct {
	Type         string `json:"type"`
	CandidateID  string `json:"candidate_id,omitempty"`
	First        string `json:"first,omitempty"`
	Second       string `json:"second,omitempty"`
	Trait        string `json:"trait,omitempty"`
	Reason       string `json:"reason,omitempty"`
	ExpectedMode string `json:"expected_mode,omitempty"`
}

type AssertionResult struct {
	Type    string `json:"type"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type CaseResult struct {
	CaseID      string            `json:"case_id"`
	ContentHash string            `json:"content_hash"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason,omitempty"`
	Assertions  []AssertionResult `json:"assertions,omitempty"`
}

type Report struct {
	ReportVersion    int          `json:"report_version"`
	CorpusHash       string       `json:"corpus_hash"`
	EvaluatorVersion string       `json:"evaluator_version"`
	Summary          Summary      `json:"summary"`
	Cases            []CaseResult `json:"cases"`
	Comparison       Comparison   `json:"comparison"`
	Limitations      []string     `json:"limitations"`
}

type Summary struct{ Passed, Failed, Ineligible, Eligible int }

func (s Summary) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{"passed": s.Passed, "failed": s.Failed, "ineligible": s.Ineligible, "eligible": s.Eligible})
}

type Comparison struct{ Improved, Regressed, NonComparable []string }

func (c Comparison) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string][]string{"improved": c.Improved, "regressed": c.Regressed, "non_comparable": c.NonComparable})
}

func LoadCorpus(path string) ([]Case, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	var cases []Case
	var rawCorpus []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var item Case
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&item); err != nil {
			return nil, "", fmt.Errorf("corpus line %d: %w", line, err)
		}
		if err := ValidateCase(item); err != nil {
			return nil, "", fmt.Errorf("corpus line %d: %w", line, err)
		}
		item.ContentHash = hashJSON(item)
		rawCorpus = append(rawCorpus, raw)
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if len(cases) == 0 {
		return nil, "", fmt.Errorf("corpus contains no cases")
	}
	sum := sha256.Sum256([]byte(strings.Join(rawCorpus, "\n") + "\n"))
	return cases, hex.EncodeToString(sum[:]), nil
}

func ValidateCase(item Case) error {
	if item.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", item.SchemaVersion)
	}
	if item.CaseID == "" || item.Mode == "" || item.Stage == "" || item.Request.Message == "" {
		return fmt.Errorf("case_id, mode, stage, and request.message are required")
	}
	if item.Mode != "album" && item.Mode != "song" {
		return fmt.Errorf("unsupported mode %q", item.Mode)
	}
	if len(item.Assertions) == 0 {
		return nil
	}
	for _, assertion := range item.Assertions {
		if assertion.Type == "" {
			return fmt.Errorf("assertion type is required")
		}
	}
	return nil
}

func Evaluate(cases []Case, corpusHash string) Report {
	return EvaluateWithPolicy(cases, corpusHash, nil, 1)
}

func EvaluateWithPolicy(cases []Case, corpusHash string, policy CandidatePolicy, seed int64) Report {
	report := Report{ReportVersion: 1, CorpusHash: corpusHash, EvaluatorVersion: EvaluatorVersion, Limitations: []string{"Frozen-plan replay does not measure fresh model interpretation or live discovery quality.", "Missing metadata is not evidence of subjective sonic correctness."}}
	for _, item := range cases {
		if policy != nil {
			item.Candidates = policy(item.Request, item.Mode, item.Candidates, seed)
		}
		result := CaseResult{CaseID: item.CaseID, ContentHash: item.ContentHash}
		if len(item.Assertions) == 0 {
			result.Status, result.Reason = "ineligible", "no explicit assertions"
			report.Summary.Ineligible++
			report.Cases = append(report.Cases, result)
			continue
		}
		result.Status, report.Summary.Eligible = "passed", report.Summary.Eligible+1
		for _, assertion := range item.Assertions {
			outcome := evaluateAssertion(item, assertion)
			result.Assertions = append(result.Assertions, outcome)
			if !outcome.Passed {
				result.Status = "failed"
			}
		}
		if result.Status == "failed" {
			report.Summary.Failed++
		} else {
			report.Summary.Passed++
		}
		report.Cases = append(report.Cases, result)
	}
	return report
}

func evaluateAssertion(item Case, assertion Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}
	switch assertion.Type {
	case "required_mode":
		result.Passed = item.Mode == assertion.ExpectedMode
		result.Message = fmt.Sprintf("mode is %s", item.Mode)
	case "candidate_present":
		result.Passed = findCandidate(item, assertion.CandidateID) >= 0
		result.Message = "candidate presence checked"
	case "excluded_candidate_absent":
		result.Passed = findCandidate(item, assertion.CandidateID) < 0 || !item.Candidates[findCandidate(item, assertion.CandidateID)].Eligible
		result.Message = "candidate exclusion checked"
	case "rank_before":
		first, second := findCandidate(item, assertion.First), findCandidate(item, assertion.Second)
		result.Passed = first >= 0 && second >= 0 && item.Candidates[first].Rank < item.Candidates[second].Rank
		result.Message = "candidate ordering checked"
	case "expected_shortfall":
		result.Passed = len(item.Candidates) == 0 && assertion.Reason != ""
		result.Message = "shortfall classification checked"
	case "required_plan_trait":
		value, ok := item.Plan[assertion.Trait]
		result.Passed = ok && value != nil
		result.Message = "plan trait checked"
	default:
		result.Message = "unsupported assertion type"
		result.Passed = false
	}
	return result
}

func findCandidate(item Case, id string) int {
	for index, candidate := range item.Candidates {
		if candidate.ID == id {
			return index
		}
	}
	return -1
}
func hashJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func Compare(current, baseline Report) Comparison {
	byID := map[string]CaseResult{}
	for _, item := range baseline.Cases {
		byID[item.CaseID] = item
	}
	var result Comparison
	for _, item := range current.Cases {
		old, ok := byID[item.CaseID]
		if !ok || old.ContentHash != item.ContentHash {
			result.NonComparable = append(result.NonComparable, item.CaseID)
			continue
		}
		if old.Status == "failed" && item.Status == "passed" {
			result.Improved = append(result.Improved, item.CaseID)
		}
		if old.Status == "passed" && item.Status == "failed" {
			result.Regressed = append(result.Regressed, item.CaseID)
		}
	}
	return result
}
