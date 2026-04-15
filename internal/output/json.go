package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bytemaster333/warpscope/internal/diagnosis"
)

// WriteJSON renders a DiagnosticReport as pretty-printed JSON.
func WriteJSON(w io.Writer, r *diagnosis.DiagnosticReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(toJSONReport(r)); err != nil {
		return fmt.Errorf("marshal report to JSON: %w", err)
	}
	return nil
}

// jsonReport is a JSON-friendly representation of DiagnosticReport.
// We define a separate struct to control field names and omit internal types.
type jsonReport struct {
	TxHash        string       `json:"txHash"`
	MessageID     string       `json:"messageID"`
	Timestamp     string       `json:"timestamp"`
	SourceChainID string       `json:"sourceChainID"`
	NetworkID     uint32       `json:"networkID"`
	PChainHeight  uint64       `json:"pChainHeight"`
	IsHealthy     bool         `json:"isHealthy"`
	RootCause     string       `json:"rootCause"`
	Summary       string       `json:"summary"`
	Checks        []jsonCheck  `json:"checks"`
}

type jsonCheck struct {
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	Passed      bool           `json:"passed"`
	Severity    string         `json:"severity"`
	Description string         `json:"description"`
	Details     map[string]any `json:"details,omitempty"`
}

func toJSONReport(r *diagnosis.DiagnosticReport) jsonReport {
	checks := make([]jsonCheck, len(r.Checks))
	for i, c := range r.Checks {
		checks[i] = jsonCheck{
			Name:        c.Name,
			Category:    string(c.Category),
			Passed:      c.Passed,
			Severity:    string(c.Severity),
			Description: c.Description,
			Details:     c.Details,
		}
	}
	return jsonReport{
		TxHash:        r.TxHash.Hex(),
		MessageID:     r.MessageID.Hex(),
		Timestamp:     r.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		SourceChainID: r.SourceChainID.String(),
		NetworkID:     r.NetworkID,
		PChainHeight:  r.PChainHeight,
		IsHealthy:     r.IsHealthy,
		RootCause:     string(r.RootCause),
		Summary:       r.Summary,
		Checks:        checks,
	}
}
