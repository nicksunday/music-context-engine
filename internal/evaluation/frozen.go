package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
)

// FrozenModelObservation is a reviewed result produced by an external,
// explicitly invoked model run. PoolHash proves that selection comparisons use
// identical candidate/context inputs; planning observations are kept separate.
type FrozenModelObservation struct {
	Model       string   `json:"model"`
	PoolHash    string   `json:"pool_hash"`
	RequestHash string   `json:"request_hash"`
	Selections  []string `json:"selections,omitempty"`
	Available   bool     `json:"available"`
	Reason      string   `json:"reason,omitempty"`
}

type FrozenModelComparison struct {
	ReportVersion   int                      `json:"report_version"`
	Evaluation      string                   `json:"evaluation"`
	PoolHash        string                   `json:"pool_hash"`
	RequestHash     string                   `json:"request_hash"`
	SelectionModels []FrozenModelObservation `json:"selection_models"`
	FreshPlanning   []FrozenModelObservation `json:"fresh_planning,omitempty"`
	Limitations     []string                 `json:"limitations"`
}

func LoadFrozenModelObservations(path string) ([]FrozenModelObservation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var observations []FrozenModelObservation
	if err := json.Unmarshal(raw, &observations); err != nil {
		return nil, err
	}
	return observations, nil
}

func CompareFrozenModelPool(observations []FrozenModelObservation) (FrozenModelComparison, error) {
	if len(observations) == 0 {
		return FrozenModelComparison{}, fmt.Errorf("no frozen model observations")
	}
	report := FrozenModelComparison{ReportVersion: 1, Evaluation: "frozen_pool_selection", Limitations: []string{"This report compares supplied observations only; it does not invoke or download models.", "Fresh planning is separate from frozen-pool selection quality.", "Unavailable models are recorded rather than silently substituted."}}
	for _, observation := range observations {
		if observation.PoolHash == "" || observation.RequestHash == "" || observation.Model == "" {
			return FrozenModelComparison{}, fmt.Errorf("frozen observation requires model, pool_hash, and request_hash")
		}
		if report.PoolHash == "" {
			report.PoolHash, report.RequestHash = observation.PoolHash, observation.RequestHash
		}
		if observation.PoolHash != report.PoolHash || observation.RequestHash != report.RequestHash {
			return FrozenModelComparison{}, fmt.Errorf("frozen observations do not share pool/request identity")
		}
		if observation.Available {
			report.SelectionModels = append(report.SelectionModels, observation)
		} else {
			report.Limitations = append(report.Limitations, observation.Model+": "+observation.Reason)
		}
	}
	return report, nil
}
