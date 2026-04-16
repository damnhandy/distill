package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveImage(t *testing.T) {
	t.Parallel()

	// helpers
	writeSpec := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "image.distill.yaml")
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}

	specWithTags := func(t *testing.T, tags ...string) string {
		t.Helper()
		var b strings.Builder
		b.WriteString("name: test-image\nbase:\n  image: debian:bookworm-slim\n  releasever: \"12\"\ncontents:\n  packages:\n    - bash\n")
		if len(tags) > 0 {
			b.WriteString("tags:\n")
			for _, tag := range tags {
				b.WriteString("  - " + tag + "\n")
			}
		}
		return writeSpec(t, b.String())
	}

	tests := []struct {
		name      string
		cmdName   string
		args      []string
		specPath  string
		wantImage string
		wantErr   string
	}{
		{
			name:      "positional arg only",
			cmdName:   "scan",
			args:      []string{"myregistry.io/app:latest"},
			wantImage: "myregistry.io/app:latest",
		},
		{
			name:      "positional arg wins over spec",
			cmdName:   "scan",
			args:      []string{"myregistry.io/app:latest"},
			wantImage: "myregistry.io/app:latest",
			// specPath set below in the test body
		},
		{
			name:      "spec with single tag",
			cmdName:   "scan",
			wantImage: "ghcr.io/damnhandy/rhel9-distilled:latest",
		},
		{
			name:      "spec with multiple tags returns first",
			cmdName:   "attest",
			wantImage: "ghcr.io/damnhandy/app:latest",
		},
		{
			name:    "spec with empty tags",
			cmdName: "scan",
			wantErr: "has no tags defined",
		},
		{
			name:    "spec path does not exist",
			cmdName: "scan",
			wantErr: "reading spec",
		},
		{
			name:    "spec with invalid YAML",
			cmdName: "scan",
			wantErr: "parsing image spec",
		},
		{
			name:    "neither arg nor spec",
			cmdName: "provenance",
			wantErr: "no image reference provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			specPath := tt.specPath
			args := tt.args

			switch tt.name {
			case "positional arg wins over spec":
				specPath = specWithTags(t, "ghcr.io/damnhandy/other:latest")
			case "spec with single tag":
				specPath = specWithTags(t, "ghcr.io/damnhandy/rhel9-distilled:latest")
			case "spec with multiple tags returns first":
				specPath = specWithTags(t, "ghcr.io/damnhandy/app:latest", "ghcr.io/damnhandy/app:1.0")
			case "spec with empty tags":
				specPath = specWithTags(t) // no tags
			case "spec path does not exist":
				specPath = "/nonexistent/path/image.distill.yaml"
			case "spec with invalid YAML":
				specPath = writeSpec(t, "this: is: not: valid: yaml: [")
			}

			got, err := resolveImage(tt.cmdName, args, specPath)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantImage, got)
		})
	}
}
