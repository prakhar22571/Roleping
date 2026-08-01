package web

import "encoding/json"

type VerdictSummary struct {
	ModelID   string `json:"model_id"`
	Match     int    `json:"match"`
	Reasoning string `json:"reasoning"`
}

// ParseVerdicts decodes the verdicts_json column (a JSON array produced by
// SQLite's json_group_array) into a slice for display. Returns nil on a
// missing/invalid value rather than erroring, since it's just a summary.
func ParseVerdicts(verdictsJSON *string) []VerdictSummary {
	if verdictsJSON == nil || *verdictsJSON == "" {
		return nil
	}
	var summaries []VerdictSummary
	if err := json.Unmarshal([]byte(*verdictsJSON), &summaries); err != nil {
		return nil
	}
	return summaries
}
