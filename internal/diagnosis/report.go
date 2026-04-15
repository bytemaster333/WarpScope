package diagnosis

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/libevm/common"
)

// CheckResult holds the outcome of a single diagnostic check.
type CheckResult struct {
	// Name is a human-readable check identifier.
	Name string

	// Category is the failure category this check detects.
	Category FailureCategory

	// Passed is true when the check did not detect a failure.
	Passed bool

	// Severity indicates how critical a failure is.
	Severity Severity

	// Description is a human-readable explanation of the result.
	Description string

	// Details contains key-value pairs with supporting evidence.
	Details map[string]any
}

// DiagnosticReport is the top-level output of the WarpScope diagnosis engine.
type DiagnosticReport struct {
	// TxHash is the source chain transaction hash that was diagnosed.
	TxHash common.Hash

	// MessageID is the Warp messageID (keccak256 of unsigned message bytes).
	MessageID common.Hash

	// Timestamp is when the diagnosis was performed.
	Timestamp time.Time

	// SourceChainID is the blockchain ID from the Warp UnsignedMessage.
	SourceChainID ids.ID

	// NetworkID is the network from the Warp UnsignedMessage.
	NetworkID uint32

	// PChainHeight is the P-Chain height used for validator set queries.
	PChainHeight uint64

	// Checks contains the ordered list of check results.
	Checks []*CheckResult

	// RootCause is the category of the first CRITICAL failure found.
	// Set to CategoryHealthy when no failures are detected.
	RootCause FailureCategory

	// IsHealthy is true when all checks pass (message was delivered successfully).
	IsHealthy bool

	// Summary is a concise one-line verdict.
	Summary string
}

// FailedChecks returns only the checks that did not pass.
func (r *DiagnosticReport) FailedChecks() []*CheckResult {
	var failed []*CheckResult
	for _, c := range r.Checks {
		if !c.Passed {
			failed = append(failed, c)
		}
	}
	return failed
}

// buildSummary assembles the one-line summary from the report state.
func buildSummary(r *DiagnosticReport) string {
	if r.IsHealthy {
		return fmt.Sprintf("Message %s delivered successfully on destination chain.", shortHash(r.MessageID))
	}
	failed := r.FailedChecks()
	if len(failed) == 0 {
		return fmt.Sprintf("Message %s: diagnosis inconclusive.", shortHash(r.MessageID))
	}
	return fmt.Sprintf("Message %s: ROOT CAUSE=%s — %s",
		shortHash(r.MessageID), r.RootCause, failed[0].Description)
}

func shortHash(h common.Hash) string {
	s := h.Hex()
	if len(s) > 10 {
		return s[:6] + "…" + s[len(s)-4:]
	}
	return s
}
