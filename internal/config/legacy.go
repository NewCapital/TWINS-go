package config

import (
	"strings"

	"github.com/sirupsen/logrus"
)

// LoadConfigAuto loads configuration from a YAML file.
// If a legacy twins.conf path is provided, it returns the default config
// with a warning instead of a hard error so the daemon can still start.
func LoadConfigAuto(path string) (*Config, error) {
	lowerPath := strings.ToLower(path)
	if strings.HasSuffix(lowerPath, ".conf") {
		logrus.Warnf("legacy twins.conf format is no longer supported, using defaults. Please migrate to twinsd.yml (YAML format): %s", path)
		return DefaultConfig(), nil
	}
	return LoadConfigWithEnvironment(path)
}
