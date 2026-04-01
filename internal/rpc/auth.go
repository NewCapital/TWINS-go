// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package rpc

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// InitializeAuthentication sets up RPC authentication (password or cookie)
// Returns the effective username and password to use
func InitializeAuthentication(config *Config, logger *logrus.Entry) (string, string, error) {
	// If username and password are explicitly set, use them
	if config.Username != "" && config.Password != "" {
		logger.Info("Using configured username/password authentication")
		return config.Username, config.Password, nil
	}

	// If cookie authentication is disabled, require username/password
	if !config.UseCookieAuth {
		return "", "", fmt.Errorf("no authentication configured: set rpcuser/rpcpassword or enable cookie auth")
	}

	// Cookie authentication enabled - check if cookie exists
	if CookieExists(config.DataDir) {
		logger.Info("Using existing cookie authentication")
		cookie, err := ReadCookieFile(config.DataDir)
		if err != nil {
			return "", "", fmt.Errorf("failed to read existing cookie: %w", err)
		}

		if err := cookie.Validate(); err != nil {
			// Cookie is invalid, regenerate
			logger.Warn("Existing cookie is invalid, regenerating")
			if err := DeleteCookieFile(config.DataDir); err != nil {
				logger.WithError(err).Warn("Failed to delete invalid cookie file")
			}
		} else {
			return cookie.Username, cookie.Password, nil
		}
	}

	// Generate new cookie
	logger.Info("Generating new authentication cookie")
	cookie, err := GenerateCookie()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate cookie: %w", err)
	}

	if err := WriteCookieFile(config.DataDir, cookie); err != nil {
		return "", "", fmt.Errorf("failed to write cookie file: %w", err)
	}

	cookiePath := GetCookiePath(config.DataDir)
	logger.Infof("Authentication cookie written to: %s", cookiePath)

	return cookie.Username, cookie.Password, nil
}

// CleanupAuthentication removes cookie file on server shutdown
func CleanupAuthentication(config *Config, logger *logrus.Entry) {
	if config.UseCookieAuth {
		if err := DeleteCookieFile(config.DataDir); err != nil {
			logger.WithError(err).Warn("Failed to delete cookie file during cleanup")
		} else {
			logger.Info("Authentication cookie file deleted")
		}
	}
}

// GetAuthCredentials returns the current authentication credentials
// This is used by CLI clients to read credentials
func GetAuthCredentials(dataDir string, username, password string) (string, string, error) {
	// If username/password provided, use them
	if username != "" && password != "" {
		return username, password, nil
	}

	// Try to read from cookie file
	cookie, err := ReadCookieFile(dataDir)
	if err != nil {
		return "", "", fmt.Errorf("no credentials provided and failed to read cookie: %w", err)
	}

	if err := cookie.Validate(); err != nil {
		return "", "", fmt.Errorf("invalid cookie: %w", err)
	}

	return cookie.Username, cookie.Password, nil
}