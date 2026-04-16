package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Finding represents a single CVE finding from a Grype scan.
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Package  string `json:"package"`
	Version  string `json:"version"`
	Fix      string `json:"fix,omitempty"`
}

// ScanJSONResult is the structured result of a ScanJSON call.
type ScanJSONResult struct {
	// Passed is true when no findings meet or exceed the failOn severity threshold.
	Passed   bool      `json:"passed"`
	Findings []Finding `json:"findings"`
}

// grype JSON output types — internal use only.
type grypeOutput struct {
	Matches []grypeMatch `json:"matches"`
}

type grypeMatch struct {
	Vulnerability grypeVuln     `json:"vulnerability"`
	Artifact      grypeArtifact `json:"artifact"`
}

type grypeVuln struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Fix      grypeFix `json:"fix"`
}

type grypeFix struct {
	Versions []string `json:"versions"`
}

type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// severityOrder maps Grype severity strings to a comparable integer level.
var severityOrder = map[string]int{
	"unknown":    0,
	"negligible": 1,
	"low":        2,
	"medium":     3,
	"high":       4,
	"critical":   5,
}

func severityLevel(s string) int {
	return severityOrder[strings.ToLower(s)]
}

// ScanJSON runs Grype against image with JSON output and returns structured
// findings. passed is true when no findings meet or exceed the failOn threshold.
// Unlike Scan, ScanJSON never fails on findings — the caller inspects Passed.
// Use "critical" as a default failOn value.
func ScanJSON(ctx context.Context, image, failOn string) (*ScanJSONResult, error) {
	if _, err := exec.LookPath("grype"); err != nil {
		return nil, fmt.Errorf("grype not found on PATH — run 'distill doctor' for install instructions")
	}

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "grype", image, "--output", "json") //nolint:gosec // G204
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr // progress output to stderr; doesn't interfere with JSON on stdout
	// Ignore exit code — non-zero only means findings were found; we parse them ourselves.
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("grype produced no output for %q", image)
	}

	var raw grypeOutput
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("parsing grype JSON output: %w", err)
	}

	threshold := severityLevel(failOn)
	if threshold == 0 {
		threshold = severityLevel("critical")
	}

	findings := make([]Finding, 0, len(raw.Matches))
	passed := true
	for _, m := range raw.Matches {
		f := Finding{
			ID:       m.Vulnerability.ID,
			Severity: m.Vulnerability.Severity,
			Package:  m.Artifact.Name,
			Version:  m.Artifact.Version,
		}
		if len(m.Vulnerability.Fix.Versions) > 0 {
			f.Fix = strings.Join(m.Vulnerability.Fix.Versions, ", ")
		}
		findings = append(findings, f)
		if severityLevel(m.Vulnerability.Severity) >= threshold {
			passed = false
		}
	}
	return &ScanJSONResult{Passed: passed, Findings: findings}, nil
}
