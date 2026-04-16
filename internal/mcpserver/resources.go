package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/damnhandy/distill/internal/spec"
)

// specSchemaJSON is the JSON Schema for ImageSpec.
// It is hand-authored from spec.go to give agents an accurate schema for
// generating and validating .distill.yaml files.
const specSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "ImageSpec",
  "description": "Declarative spec for a distill .distill.yaml image build",
  "type": "object",
  "required": ["name", "source", "contents"],
  "properties": {
    "name": {
      "type": "string",
      "description": "Human-readable image name (required)"
    },
    "description": {
      "type": "string",
      "description": "Optional description of the image's purpose"
    },
    "variant": {
      "type": "string",
      "enum": ["runtime", "dev"],
      "default": "runtime",
      "description": "runtime removes the package manager from the final image; dev retains it"
    },
    "platforms": {
      "type": "array",
      "items": { "type": "string" },
      "default": ["linux/amd64", "linux/arm64"],
      "description": "Target build platforms. Defaults to [linux/amd64, linux/arm64] when omitted."
    },
    "source": {
      "type": "object",
      "required": ["image", "releasever"],
      "description": "Source distribution image used for the chroot bootstrap",
      "properties": {
        "image": {
          "type": "string",
          "description": "OCI image reference (e.g. registry.access.redhat.com/ubi9/ubi)"
        },
        "releasever": {
          "type": "string",
          "description": "Distribution release version (e.g. \"9\" for RHEL9, \"bookworm\" for Debian)"
        },
        "packageManager": {
          "type": "string",
          "enum": ["dnf", "apt"],
          "description": "Package manager (auto-inferred from source.image if omitted)"
        }
      }
    },
    "destination": {
      "type": "object",
      "required": ["image"],
      "description": "OCI image reference for the built image",
      "properties": {
        "image": {
          "type": "string",
          "description": "Registry and image name without tag (e.g. myregistry.io/myapp)"
        },
        "releasever": {
          "type": "string",
          "default": "latest",
          "description": "Tag to apply to the image (defaults to \"latest\")"
        }
      }
    },
    "contents": {
      "type": "object",
      "required": ["packages"],
      "properties": {
        "packages": {
          "type": "array",
          "minItems": 1,
          "items": { "type": "string" },
          "description": "List of packages to install (at least one required)"
        }
      }
    },
    "accounts": {
      "type": "object",
      "description": "Non-root users and groups to create inside the image",
      "properties": {
        "run-as": {
          "type": "string",
          "description": "Username to run the container as (USER directive)"
        },
        "users": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name", "uid", "gid"],
            "properties": {
              "name": { "type": "string" },
              "uid":  { "type": "integer" },
              "gid":  { "type": "integer" },
              "shell": {
                "type": "string",
                "default": "/sbin/nologin"
              },
              "groups": {
                "type": "array",
                "items": { "type": "string" }
              }
            }
          }
        },
        "groups": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name", "gid"],
            "properties": {
              "name":    { "type": "string" },
              "gid":     { "type": "integer" },
              "members": { "type": "array", "items": { "type": "string" } }
            }
          }
        }
      }
    },
    "environment": {
      "type": "object",
      "additionalProperties": { "type": "string" },
      "description": "Environment variables (ENV in Dockerfile)"
    },
    "entrypoint": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Container entrypoint (ENTRYPOINT in Dockerfile)"
    },
    "cmd": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Default command (CMD in Dockerfile)"
    },
    "work-dir": {
      "type": "string",
      "description": "Working directory (WORKDIR in Dockerfile)"
    },
    "annotations": {
      "type": "object",
      "additionalProperties": { "type": "string" },
      "description": "OCI image labels (LABEL in Dockerfile)"
    },
    "volumes": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Mount points (VOLUME in Dockerfile)"
    },
    "ports": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Exposed ports (EXPOSE in Dockerfile), e.g. \"8080/tcp\""
    },
    "paths": {
      "type": "array",
      "description": "Filesystem entries to create in the image chroot",
      "items": {
        "type": "object",
        "required": ["type", "path"],
        "properties": {
          "type": {
            "type": "string",
            "enum": ["directory", "file", "symlink"]
          },
          "path":    { "type": "string" },
          "source":  { "type": "string", "description": "Symlink target (for type=symlink)" },
          "content": { "type": "string", "description": "File content (for type=file)" },
          "uid":     { "type": "integer" },
          "gid":     { "type": "integer" },
          "mode":    { "type": "string", "description": "Octal permissions (e.g. \"0755\")" }
        }
      }
    },
    "pipeline": {
      "type": "object",
      "description": "Supply-chain steps that run with distill build --pipeline or distill publish",
      "properties": {
        "scan": {
          "type": "object",
          "properties": {
            "enabled":  { "type": "boolean" },
            "fail-on": {
              "type": "string",
              "enum": ["critical", "high", "medium", "low", "negligible"],
              "default": "critical"
            }
          }
        },
        "sbom": {
          "type": "object",
          "properties": {
            "enabled": { "type": "boolean" },
            "output":  { "type": "string", "default": "sbom.spdx.json" }
          }
        },
        "provenance": {
          "type": "object",
          "properties": {
            "enabled":   { "type": "boolean" },
            "predicate": { "type": "string", "description": "Path to write predicate JSON" }
          }
        }
      }
    }
  }
}`

// baseEntry is the shape of one entry in the distill://bases resource.
type baseEntry struct {
	Key            string   `json:"key"`
	Image          string   `json:"image"`
	Releasever     string   `json:"releasever"`
	PackageManager string   `json:"package_manager"`
	DefaultShell   string   `json:"default_shell"`
	DefaultCmd     string   `json:"default_cmd"`
	SamplePackages []string `json:"sample_packages"`
}

// handleSpecSchema serves the ImageSpec JSON Schema.
func handleSpecSchema(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "distill://spec/schema",
			MIMEType: "application/schema+json",
			Text:     specSchemaJSON,
		},
	}, nil
}

// handleBases serves the list of supported base distributions.
func handleBases(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	entries := make([]baseEntry, 0, len(spec.BasePresets))
	for key, p := range spec.BasePresets {
		entries = append(entries, baseEntry{
			Key:            key,
			Image:          p.Image,
			Releasever:     p.Releasever,
			PackageManager: p.PackageManager,
			DefaultShell:   p.DefaultShell,
			DefaultCmd:     p.DefaultCmd,
			SamplePackages: p.SamplePackages,
		})
	}

	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "distill://bases",
			MIMEType: "application/json",
			Text:     string(raw),
		},
	}, nil
}
