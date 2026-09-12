package evaluation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type LiveObservation struct {
	CaseID          string           `json:"case_id"`
	Mode            string           `json:"mode"`
	Request         string           `json:"request"`
	Model           string           `json:"model"`
	Application     string           `json:"application_version"`
	FitJudgment     string           `json:"fit_judgment"`
	ReturnedCount   int              `json:"returned_count"`
	RequestedLimit  int              `json:"requested_limit"`
	Empty           bool             `json:"empty"`
	ProviderFailure string           `json:"provider_failure,omitempty"`
	TotalMS         int64            `json:"total_ms"`
	StageMS         map[string]int64 `json:"stage_ms,omitempty"`
	CallCounts      map[string]int   `json:"call_counts,omitempty"`
	Cohort          string           `json:"cohort,omitempty"`
	SettingsHash    string           `json:"settings_hash,omitempty"`
}

type LiveSummary struct {
	Observations     int     `json:"observations"`
	FitJudged        int     `json:"fit_judged"`
	FitMet           int     `json:"fit_met"`
	Empty            int     `json:"empty"`
	ProviderFailures int     `json:"provider_failures"`
	TotalMedianMS    float64 `json:"total_median_ms"`
	TotalP95MS       float64 `json:"total_p95_ms"`
}

type LiveReport struct {
	ReportVersion int               `json:"report_version"`
	Evaluation    string            `json:"evaluation"`
	Observations  []LiveObservation `json:"observations"`
	Summary       LiveSummary       `json:"summary"`
	Limitations   []string          `json:"limitations"`
}

type BeforeAfterReport struct {
	ReportVersion int         `json:"report_version"`
	Evaluation    string      `json:"evaluation"`
	Comparable    bool        `json:"comparable"`
	Before        LiveSummary `json:"before"`
	After         LiveSummary `json:"after"`
	Limitations   []string    `json:"limitations"`
}

func CompareBeforeAfter(before, after []LiveObservation) BeforeAfterReport {
	report := BeforeAfterReport{ReportVersion: 1, Evaluation: "live_manual_before_after", Limitations: []string{"Fit judgments remain subjective and are not automated quality assertions."}}
	if len(before) == 0 || len(after) == 0 {
		report.Limitations = append(report.Limitations, "Both cohorts require at least one observation.")
		return report
	}
	settings := before[0].SettingsHash
	report.Comparable = settings != ""
	for _, observation := range before {
		if observation.SettingsHash != settings {
			report.Comparable = false
		}
	}
	for _, observation := range after {
		if observation.SettingsHash != settings {
			report.Comparable = false
		}
	}
	if !report.Comparable {
		report.Limitations = append(report.Limitations, "Before and after observations do not share a verified settings hash; no improvement claim is made.")
	}
	report.Before = SummarizeLiveObservations(before).Summary
	report.After = SummarizeLiveObservations(after).Summary
	return report
}

func LoadLiveObservations(path string) ([]LiveObservation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var result []LiveObservation
	for scanner.Scan() {
		var observation LiveObservation
		if err := json.Unmarshal(scanner.Bytes(), &observation); err != nil {
			return nil, err
		}
		if observation.CaseID == "" || observation.Request == "" || observation.Mode == "" {
			return nil, fmt.Errorf("live observation requires case_id, mode, and request")
		}
		result = append(result, observation)
	}
	return result, scanner.Err()
}

func SummarizeLiveObservations(observations []LiveObservation) LiveReport {
	report := LiveReport{ReportVersion: 1, Evaluation: "live_manual", Observations: observations, Limitations: []string{"Observations are explicitly supplied live/manual measurements, not deterministic replay.", "Subjective fit judgments are reported separately from provider/model failures.", "No network or model call is performed by this report command."}}
	values := make([]int64, 0, len(observations))
	for _, observation := range observations {
		report.Summary.Observations++
		if observation.FitJudgment != "" {
			report.Summary.FitJudged++
			if observation.FitJudgment == "met" {
				report.Summary.FitMet++
			}
		}
		if observation.Empty || observation.ReturnedCount == 0 {
			report.Summary.Empty++
		}
		if observation.ProviderFailure != "" {
			report.Summary.ProviderFailures++
		}
		values = append(values, observation.TotalMS)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values) > 0 {
		report.Summary.TotalMedianMS = percentile(values, .50)
		report.Summary.TotalP95MS = percentile(values, .95)
	}
	return report
}

func percentile(values []int64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return float64(values[index])
}
