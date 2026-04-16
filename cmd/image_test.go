package cmd

import (
	"os"
	"path/filepath"
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

	baseSpec := "name: test-image\nsource:\n  image: debian:bookworm-slim\n  releasever: \"bookworm\"\ncontents:\n  packages:\n    - bash\n"

	specWithDestination := func(t *testing.T, image, releasever string) string {
		t.Helper()
		s := baseSpec
		if image != "" {
			s += "destination:\n  image: " + image + "\n"
			if releasever != "" {
				s += "  releasever: " + releasever + "\n"
			}
		}
		return writeSpec(t, s)
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
			name:      "spec with destination image and releasever",
			cmdName:   "scan",
			wantImage: "ghcr.io/damnhandy/rhel9-distilled:latest",
		},
		{
			name:      "spec with destination image no releasever defaults to latest",
			cmdName:   "attest",
			wantImage: "ghcr.io/damnhandy/app:latest",
		},
		{
			name:    "spec with no destination",
			cmdName: "scan",
			wantErr: "has no destination defined",
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
				specPath = specWithDestination(t, "ghcr.io/damnhandy/other", "latest")
			case "spec with destination image and releasever":
				specPath = specWithDestination(t, "ghcr.io/damnhandy/rhel9-distilled", "latest")
			case "spec with destination image no releasever defaults to latest":
				specPath = specWithDestination(t, "ghcr.io/damnhandy/app", "")
			case "spec with no destination":
				specPath = specWithDestination(t, "", "")
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
