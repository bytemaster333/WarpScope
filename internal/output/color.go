// Package output provides terminal output formatters for DiagnosticReports.
package output

import (
	"github.com/bytemaster333/warpscope/internal/diagnosis"
	"github.com/fatih/color"
)

var (
	colorPass    = color.New(color.FgGreen, color.Bold)
	colorFail    = color.New(color.FgRed, color.Bold)
	colorWarn    = color.New(color.FgYellow, color.Bold)
	colorInfo    = color.New(color.FgCyan)
	colorHeader  = color.New(color.FgWhite, color.Bold)
	colorHealthy = color.New(color.FgGreen, color.Bold)
	colorUnhealthy = color.New(color.FgRed, color.Bold)
)

// StatusIcon returns a colored status icon for a check result.
func StatusIcon(c *diagnosis.CheckResult) string {
	if c.Passed {
		return colorPass.Sprint("✓")
	}
	switch c.Severity {
	case diagnosis.SeverityCritical:
		return colorFail.Sprint("✗")
	case diagnosis.SeverityWarning:
		return colorWarn.Sprint("!")
	default:
		return colorInfo.Sprint("i")
	}
}

// SeverityColored returns a colored severity string.
func SeverityColored(s diagnosis.Severity) string {
	switch s {
	case diagnosis.SeverityCritical:
		return colorFail.Sprint(string(s))
	case diagnosis.SeverityWarning:
		return colorWarn.Sprint(string(s))
	default:
		return colorInfo.Sprint(string(s))
	}
}

// HealthColored returns a colored verdict string.
func HealthColored(healthy bool) string {
	if healthy {
		return colorHealthy.Sprint("HEALTHY")
	}
	return colorUnhealthy.Sprint("FAILED")
}
