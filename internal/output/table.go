package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bytemaster333/warpscope/internal/diagnosis"
	"github.com/olekukonko/tablewriter"
)

// WriteTable renders a DiagnosticReport as a human-readable terminal table.
func WriteTable(w io.Writer, r *diagnosis.DiagnosticReport) {
	// ── Header ────────────────────────────────────────────────────────────────
	fmt.Fprintln(w)
	colorHeader.Fprintf(w, "  WarpScope Diagnostic Report\n")
	fmt.Fprintf(w, "  %-20s %s\n", "Transaction:", r.TxHash.Hex())
	fmt.Fprintf(w, "  %-20s %s\n", "Message ID:", r.MessageID.Hex())
	fmt.Fprintf(w, "  %-20s %s\n", "Timestamp:", r.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, "  %-20s %s\n", "Network ID:", networkName(r.NetworkID))
	if r.PChainHeight > 0 {
		fmt.Fprintf(w, "  %-20s %d\n", "P-Chain Height:", r.PChainHeight)
	}
	fmt.Fprintf(w, "  %-20s %s\n", "Verdict:", HealthColored(r.IsHealthy))
	if !r.IsHealthy {
		fmt.Fprintf(w, "  %-20s %s\n", "Root Cause:", colorFail.Sprint(string(r.RootCause)))
	}
	fmt.Fprintln(w)

	// ── Checks table ─────────────────────────────────────────────────────────
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"", "Check", "Status", "Severity", "Description"})
	table.SetBorder(false)
	table.SetColumnSeparator("  ")
	table.SetHeaderLine(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(true)
	table.SetColWidth(60)

	for _, c := range r.Checks {
		icon := StatusIcon(c)
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		// Truncate long descriptions for table display.
		desc := c.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		table.Append([]string{icon, c.Name, status, string(c.Severity), desc})
	}
	table.Render()

	// ── Summary ───────────────────────────────────────────────────────────────
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", r.Summary)

	// ── Failed check details ──────────────────────────────────────────────────
	failed := r.FailedChecks()
	if len(failed) > 0 {
		fmt.Fprintln(w)
		colorHeader.Fprintf(w, "  Failure Details\n")
		fmt.Fprintln(w, strings.Repeat("─", 72))
		for _, c := range failed {
			fmt.Fprintf(w, "  [%s] %s\n", colorFail.Sprint(string(c.Category)), c.Name)
			fmt.Fprintf(w, "  %s\n", c.Description)
			if len(c.Details) > 0 {
				for k, v := range c.Details {
					fmt.Fprintf(w, "    %-30s %v\n", k+":", v)
				}
			}
			fmt.Fprintln(w)
		}
	}
}

func networkName(id uint32) string {
	switch id {
	case 1:
		return "Mainnet (1)"
	case 5:
		return "Fuji (5)"
	default:
		return fmt.Sprintf("unknown (%d)", id)
	}
}
