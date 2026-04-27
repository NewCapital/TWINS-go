package cli

import (
	"testing"

	urfave "github.com/urfave/cli/v2"
)

// TestTLSFlagsExist verifies all 8 TLS-related flags are present in CommonDaemonFlags.
func TestTLSFlagsExist(t *testing.T) {
	flags := CommonDaemonFlags()

	expectedFlags := []string{
		"rpc-tls-enabled",
		"rpc-tls-cert",
		"rpc-tls-key",
		"rpc-tls-expiry-warn-days",
		"rpc-tls-reload-passphrase-file",
		"rpc-tls-mtls-enabled",
		"rpc-tls-mtls-client-ca",
		"rpc-allow-plaintext-public",
	}

	// Build a set of flag names for lookup
	flagNames := make(map[string]bool)
	for _, f := range flags {
		for _, name := range f.Names() {
			flagNames[name] = true
		}
	}

	for _, expected := range expectedFlags {
		if !flagNames[expected] {
			t.Errorf("expected flag --%s not found in CommonDaemonFlags()", expected)
		}
	}
}

// TestLegacyRPCSSLFlagsHidden verifies the 4 legacy --rpcssl* flags are present
// and marked as Hidden so they don't appear in --help output.
func TestLegacyRPCSSLFlagsHidden(t *testing.T) {
	flags := CommonDaemonFlags()

	legacyFlags := []string{
		"rpcssl",
		"rpcsslcertificatechainfile",
		"rpcsslprivatekeyfile",
		"rpcsslciphers",
	}

	// Build a map of flag name → concrete flag for hidden check
	type hiddenChecker interface {
		Names() []string
	}
	flagMap := make(map[string]urfave.Flag)
	for _, f := range flags {
		names := f.Names()
		if len(names) > 0 {
			flagMap[names[0]] = f
		}
	}

	for _, legacy := range legacyFlags {
		f, exists := flagMap[legacy]
		if !exists {
			t.Errorf("legacy flag --%s not found in CommonDaemonFlags()", legacy)
			continue
		}
		// Type-assert to concrete types to check Hidden field
		switch ff := f.(type) {
		case *urfave.BoolFlag:
			if !ff.Hidden {
				t.Errorf("legacy flag --%s should be Hidden but is visible", legacy)
			}
		case *urfave.StringFlag:
			if !ff.Hidden {
				t.Errorf("legacy flag --%s should be Hidden but is visible", legacy)
			}
		default:
			t.Errorf("legacy flag --%s has unexpected type %T", legacy, f)
		}
	}
}

// TestTLSFlagDefaults verifies default values for TLS flags.
func TestTLSFlagDefaults(t *testing.T) {
	flags := CommonDaemonFlags()

	flagMap := make(map[string]urfave.Flag)
	for _, f := range flags {
		names := f.Names()
		if len(names) > 0 {
			flagMap[names[0]] = f
		}
	}

	// Bool flags should default to false
	boolDefaults := map[string]bool{
		"rpc-tls-enabled":            false,
		"rpc-tls-mtls-enabled":       false,
		"rpc-allow-plaintext-public": false,
	}
	for name, expected := range boolDefaults {
		f, ok := flagMap[name]
		if !ok {
			t.Errorf("flag --%s not found", name)
			continue
		}
		bf, ok := f.(*urfave.BoolFlag)
		if !ok {
			t.Errorf("flag --%s is not a BoolFlag", name)
			continue
		}
		if bf.Value != expected {
			t.Errorf("flag --%s default = %v, want %v", name, bf.Value, expected)
		}
	}

	// Int flag default
	f, ok := flagMap["rpc-tls-expiry-warn-days"]
	if !ok {
		t.Fatal("flag --rpc-tls-expiry-warn-days not found")
	}
	intFlag, ok := f.(*urfave.IntFlag)
	if !ok {
		t.Fatal("flag --rpc-tls-expiry-warn-days is not an IntFlag")
	}
	if intFlag.Value != 30 {
		t.Errorf("flag --rpc-tls-expiry-warn-days default = %d, want 30", intFlag.Value)
	}
}
