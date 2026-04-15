package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/damnhandy/distill/internal/spec"
)

const (
	slsaBuilderIDURI = "https://github.com/damnhandy/distill"
	slsaBuildTypeURI = "https://github.com/damnhandy/distill/buildType@v1"
)

// ProvenanceOptions configures SLSA provenance generation for Provenance.
type ProvenanceOptions struct {
	// SpecPath is the path to the image.yaml spec used during the build.
	// When set, enriches the provenance with configSource and base-image materials.
	SpecPath string

	// PredicatePath writes the predicate JSON to this file instead of a temp file.
	// Useful for auditing or archiving the raw predicate.
	PredicatePath string
}

// Provenance generates a SLSA v0.2 provenance predicate for image and attaches
// it as a cosign attestation using keyless (Sigstore) signing. If opts.SpecPath
// is provided the predicate is enriched with configSource and base-image materials.
func Provenance(ctx context.Context, image string, opts ProvenanceOptions) error {
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign not found on PATH — install with: devbox shell")
	}

	now := time.Now().UTC()
	predicate := slsaPredicate{
		Builder:   slsaBuilderRef{ID: slsaBuilderIDURI},
		BuildType: slsaBuildTypeURI,
		Metadata: slsaMetadata{
			BuildStartedOn:  now,
			BuildFinishedOn: now,
			Completeness: slsaCompleteness{
				Parameters:  opts.SpecPath != "",
				Environment: false,
				Materials:   false,
			},
			Reproducible: false,
		},
	}

	if opts.SpecPath != "" {
		if err := enrichFromSpec(ctx, &predicate, opts.SpecPath); err != nil {
			// Non-fatal: warn and continue with a minimal predicate.
			fmt.Fprintf(os.Stderr, "warning: could not enrich provenance from spec: %v\n", err)
		}
	}

	predicatePath := opts.PredicatePath
	cleanupTemp := false
	if predicatePath == "" {
		f, err := os.CreateTemp("", "distill-provenance-*.json")
		if err != nil {
			return fmt.Errorf("creating temp predicate file: %w", err)
		}
		predicatePath = f.Name()
		if err := f.Close(); err != nil {
			return fmt.Errorf("closing temp predicate file: %w", err)
		}
		cleanupTemp = true
	}
	if cleanupTemp {
		defer func() {
			if err := os.Remove(predicatePath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove temp predicate file %s: %v\n", predicatePath, err)
			}
		}()
	}

	data, err := json.MarshalIndent(predicate, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling provenance predicate: %w", err)
	}
	if err := os.WriteFile(predicatePath, data, 0o600); err != nil {
		return fmt.Errorf("writing predicate to %s: %w", predicatePath, err)
	}

	return run(ctx, os.Stdout,
		"cosign", "attest",
		"--predicate", predicatePath,
		"--type", "slsaprovenance",
		"--yes",
		image,
	)
}

// enrichFromSpec parses the spec file and adds configSource, parameters, and
// materials to the predicate.
func enrichFromSpec(ctx context.Context, p *slsaPredicate, specPath string) error {
	absPath, err := filepath.Abs(specPath)
	if err != nil {
		return fmt.Errorf("resolving spec path: %w", err)
	}

	data, err := os.ReadFile(absPath) //nolint:gosec // G304: specPath is a validated CLI argument
	if err != nil {
		return fmt.Errorf("reading spec file: %w", err)
	}

	sum := sha256.Sum256(data)
	specDigest := hex.EncodeToString(sum[:])

	p.Invocation.ConfigSource = slsaConfigSource{
		URI:    "file://" + absPath,
		Digest: slsaDigestSet{"sha256": specDigest},
	}

	s, err := spec.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing spec: %w", err)
	}

	if p.Invocation.Parameters == nil {
		p.Invocation.Parameters = map[string]string{}
	}
	p.Invocation.Parameters["baseImage"] = s.Base.Image

	material := slsaMaterial{URI: s.Base.Image}
	if digest, err := imageDigest(ctx, s.Base.Image); err == nil {
		material.Digest = slsaDigestSet{"sha256": digest}
		p.Metadata.Completeness.Materials = true
	} else {
		fmt.Fprintf(os.Stderr, "warning: could not resolve base image digest for %s: %v\n", s.Base.Image, err)
	}
	p.Materials = []slsaMaterial{material}

	return nil
}

// imageDigest returns the sha256 hex digest (without "sha256:" prefix) for
// the given OCI image reference using skopeo inspect.
func imageDigest(ctx context.Context, image string) (string, error) {
	if _, err := exec.LookPath("skopeo"); err != nil {
		return "", fmt.Errorf("skopeo not found on PATH")
	}
	out, err := exec.CommandContext(ctx, "skopeo", "inspect", "docker://"+image).Output() //nolint:gosec // G204: image is sourced from the validated spec base.image field
	if err != nil {
		return "", fmt.Errorf("skopeo inspect: %w", err)
	}
	var result struct {
		Digest string `json:"Digest"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing skopeo inspect output: %w", err)
	}
	return strings.TrimPrefix(result.Digest, "sha256:"), nil
}

// ── SLSA v0.2 predicate types ────────────────────────────────────────────────

// slsaDigestSet maps algorithm names to hex-encoded digest values.
// e.g. {"sha256": "abc123..."}.
type slsaDigestSet map[string]string

type slsaBuilderRef struct {
	ID string `json:"id"`
}

type slsaConfigSource struct {
	URI    string        `json:"uri,omitempty"`
	Digest slsaDigestSet `json:"digest,omitempty"`
}

type slsaInvocation struct {
	ConfigSource slsaConfigSource  `json:"configSource,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
}

type slsaCompleteness struct {
	Parameters  bool `json:"parameters"`
	Environment bool `json:"environment"`
	Materials   bool `json:"materials"`
}

type slsaMetadata struct {
	BuildStartedOn  time.Time        `json:"buildStartedOn"`
	BuildFinishedOn time.Time        `json:"buildFinishedOn"`
	Completeness    slsaCompleteness `json:"completeness"`
	Reproducible    bool             `json:"reproducible"`
}

type slsaMaterial struct {
	URI    string        `json:"uri"`
	Digest slsaDigestSet `json:"digest,omitempty"`
}

type slsaPredicate struct {
	Builder    slsaBuilderRef `json:"builder"`
	BuildType  string         `json:"buildType"`
	Invocation slsaInvocation `json:"invocation,omitempty"`
	Metadata   slsaMetadata   `json:"metadata"`
	Materials  []slsaMaterial `json:"materials,omitempty"`
}
