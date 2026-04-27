package main

import (
	"flag"
	"strings"
	"testing"

	twinslib "github.com/twins-dev/twins-core/internal/cli"
	"github.com/urfave/cli/v2"
)

// newTestContext creates a cli.Context with the given flags set for testing.
func newTestContext(flags []cli.Flag, args ...string) *cli.Context {
	app := &cli.App{
		Flags: flags,
	}

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range flags {
		if err := f.Apply(set); err != nil {
			panic(err)
		}
	}

	if err := set.Parse(args); err != nil {
		panic(err)
	}

	return cli.NewContext(app, set, nil)
}

// TestRejectLegacyRPCSSLFlags verifies that each legacy --rpcssl* flag is rejected
// with an error message containing the modern equivalent.
func TestRejectLegacyRPCSSLFlags(t *testing.T) {
	tests := []struct {
		flag    string
		args    []string
		wantErr string
	}{
		{
			flag:    "rpcssl",
			args:    []string{"--rpcssl"},
			wantErr: "--rpc-tls-enabled",
		},
		{
			flag:    "rpcsslcertificatechainfile",
			args:    []string{"--rpcsslcertificatechainfile=cert.pem"},
			wantErr: "--rpc-tls-cert",
		},
		{
			flag:    "rpcsslprivatekeyfile",
			args:    []string{"--rpcsslprivatekeyfile=key.pem"},
			wantErr: "--rpc-tls-key",
		},
		{
			flag:    "rpcsslciphers",
			args:    []string{"--rpcsslciphers=AES256"},
			wantErr: "TLS 1.3 manages cipher selection automatically",
		},
	}

	daemonFlags := twinslib.CommonDaemonFlags()

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			ctx := newTestContext(daemonFlags, tt.args...)
			err := rejectLegacyRPCSSLFlags(ctx)
			if err == nil {
				t.Fatalf("expected error for --%s, got nil", tt.flag)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if !strings.Contains(err.Error(), "no longer supported") {
				t.Errorf("error %q should contain 'no longer supported'", err.Error())
			}
		})
	}
}

// TestRejectLegacyRPCSSLFlags_NoLegacy verifies no error when legacy flags are not set.
func TestRejectLegacyRPCSSLFlags_NoLegacy(t *testing.T) {
	daemonFlags := twinslib.CommonDaemonFlags()
	ctx := newTestContext(daemonFlags) // no args
	err := rejectLegacyRPCSSLFlags(ctx)
	if err != nil {
		t.Fatalf("expected no error with no legacy flags, got: %v", err)
	}
}

// TestBuildConfigManagerTLSFlags verifies that TLS CLI flags are wired correctly
// to the config via buildConfigManager.
func TestBuildConfigManagerTLSFlags(t *testing.T) {
	daemonFlags := twinslib.CommonDaemonFlags()

	// Create a temp dir for data and config
	tmpDir := t.TempDir()

	ctx := newTestContext(daemonFlags,
		"--rpc-tls-enabled",
		"--rpc-tls-cert=/path/to/cert.pem",
		"--rpc-tls-key=/path/to/key.pem",
		"--rpc-tls-expiry-warn-days=14",
		"--rpc-tls-reload-passphrase-file=/path/to/hash",
		"--rpc-tls-mtls-enabled",
		"--rpc-tls-mtls-client-ca=/path/to/ca.pem",
		"--rpc-allow-plaintext-public",
	)

	cm, _, err := buildConfigManager(ctx, tmpDir)
	if err != nil {
		t.Fatalf("buildConfigManager failed: %v", err)
	}

	// Verify TLS values were applied
	snap := cm.Snapshot()

	if !snap.RPC.TLS.Enabled {
		t.Error("rpc.tls.enabled should be true")
	}
	if snap.RPC.TLS.CertFile != "/path/to/cert.pem" {
		t.Errorf("rpc.tls.certFile = %q, want /path/to/cert.pem", snap.RPC.TLS.CertFile)
	}
	if snap.RPC.TLS.KeyFile != "/path/to/key.pem" {
		t.Errorf("rpc.tls.keyFile = %q, want /path/to/key.pem", snap.RPC.TLS.KeyFile)
	}
	if snap.RPC.TLS.ExpiryWarnDays != 14 {
		t.Errorf("rpc.tls.expiryWarnDays = %d, want 14", snap.RPC.TLS.ExpiryWarnDays)
	}
	if snap.RPC.TLS.ReloadPassphraseFile != "/path/to/hash" {
		t.Errorf("rpc.tls.reloadPassphraseFile = %q, want /path/to/hash", snap.RPC.TLS.ReloadPassphraseFile)
	}
	if !snap.RPC.TLS.MTLS.Enabled {
		t.Error("rpc.tls.mtls.enabled should be true")
	}
	if snap.RPC.TLS.MTLS.ClientCAFile != "/path/to/ca.pem" {
		t.Errorf("rpc.tls.mtls.clientCAFile = %q, want /path/to/ca.pem", snap.RPC.TLS.MTLS.ClientCAFile)
	}
	if !snap.RPC.AllowPlaintextPublic {
		t.Error("rpc.allowPlaintextPublic should be true")
	}

	// Verify CLI-locked status
	for _, key := range []string{
		"rpc.tls.enabled", "rpc.tls.certFile", "rpc.tls.keyFile",
		"rpc.tls.expiryWarnDays", "rpc.tls.reloadPassphraseFile",
		"rpc.tls.mtls.enabled", "rpc.tls.mtls.clientCAFile",
		"rpc.allowPlaintextPublic",
	} {
		if !cm.IsLocked(key) {
			t.Errorf("key %q should be CLI-locked", key)
		}
	}
}

// TestBuildConfigManagerTLSDefaults verifies that TLS config keeps defaults
// when no TLS flags are passed.
func TestBuildConfigManagerTLSDefaults(t *testing.T) {
	daemonFlags := twinslib.CommonDaemonFlags()
	tmpDir := t.TempDir()

	ctx := newTestContext(daemonFlags) // no TLS args
	cm, _, err := buildConfigManager(ctx, tmpDir)
	if err != nil {
		t.Fatalf("buildConfigManager failed: %v", err)
	}

	snap := cm.Snapshot()

	if snap.RPC.TLS.Enabled {
		t.Error("rpc.tls.enabled should default to false")
	}
	if snap.RPC.TLS.CertFile != "" {
		t.Errorf("rpc.tls.certFile should default to empty, got %q", snap.RPC.TLS.CertFile)
	}
	if snap.RPC.TLS.ExpiryWarnDays != 30 {
		t.Errorf("rpc.tls.expiryWarnDays should default to 30, got %d", snap.RPC.TLS.ExpiryWarnDays)
	}
	if snap.RPC.AllowPlaintextPublic {
		t.Error("rpc.allowPlaintextPublic should default to false")
	}

	// Keys should NOT be locked when flags not explicitly set
	for _, key := range []string{
		"rpc.tls.enabled", "rpc.tls.certFile", "rpc.allowPlaintextPublic",
	} {
		if cm.IsLocked(key) {
			t.Errorf("key %q should not be CLI-locked when flag not set", key)
		}
	}
}

// TestBuildConfigManagerLegacyRejection verifies buildConfigManager rejects
// legacy --rpcssl flags before applying any other settings.
func TestBuildConfigManagerLegacyRejection(t *testing.T) {
	daemonFlags := twinslib.CommonDaemonFlags()
	tmpDir := t.TempDir()

	ctx := newTestContext(daemonFlags, "--rpcssl")
	_, _, err := buildConfigManager(ctx, tmpDir)
	if err == nil {
		t.Fatal("expected error for --rpcssl, got nil")
	}
	if !strings.Contains(err.Error(), "--rpc-tls-enabled") {
		t.Errorf("error %q should mention --rpc-tls-enabled", err.Error())
	}
}
