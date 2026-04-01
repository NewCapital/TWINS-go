package utils

import (
	"strconv"
	"strings"

	"github.com/twins-dev/twins-core/internal/cli"
)

// Version represents the application version information
type Version struct {
	Major      int    `json:"major"`
	Minor      int    `json:"minor"`
	Patch      int    `json:"patch"`
	Prerelease string `json:"prerelease"`
	Build      string `json:"build"`
	Codename   string `json:"codename"`
}

// String returns the version as a semantic version string
func (v *Version) String() string {
	return cli.Version
}

// GetVersionOrDefault returns version parsed from internal/cli/version.go
func GetVersionOrDefault() *Version {
	// Parse version from cli.Version (e.g., "4.0.2")
	parts := strings.Split(cli.Version, ".")

	major, minor, patch := 0, 0, 0
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		// Handle potential prerelease suffix (e.g., "2-beta")
		patchStr := strings.Split(parts[2], "-")[0]
		patch, _ = strconv.Atoi(patchStr)
	}

	// Extract prerelease if present (e.g., "4.0.2-beta" -> "beta")
	prerelease := ""
	if idx := strings.Index(cli.Version, "-"); idx != -1 {
		prerelease = cli.Version[idx+1:]
	}

	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		Build:      cli.GitCommit,
		Codename:   "TWINS Core",
	}
}
