package cli

import (
	"runtime"
	"testing"
)

func TestNormalizeArgs_Empty(t *testing.T) {
	args := []string{"program"}
	result := NormalizeArgs(args)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestNormalizeArgs_DoubleHyphen(t *testing.T) {
	args := []string{"program", "--testnet", "--datadir=/path"}
	result := NormalizeArgs(args)

	expected := []string{"-testnet", "-datadir=/path"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(result), result)
	}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("arg %d: expected %q, got %q", i, exp, result[i])
		}
	}
}

func TestNormalizeArgs_SingleHyphen(t *testing.T) {
	args := []string{"program", "-testnet", "-datadir=/path"}
	result := NormalizeArgs(args)

	expected := []string{"-testnet", "-datadir=/path"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(result), result)
	}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("arg %d: expected %q, got %q", i, exp, result[i])
		}
	}
}

func TestNormalizeArgs_WindowsSlash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	args := []string{"program", "/testnet", "/datadir=C:\\path"}
	result := NormalizeArgs(args)

	// Windows converts to lowercase
	if len(result) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(result), result)
	}
	if result[0] != "-testnet" {
		t.Errorf("expected -testnet, got %q", result[0])
	}
}

func TestProcessNegativeFlags_NoSplash(t *testing.T) {
	args := []string{"-nosplash", "-testnet"}
	result, negated := ProcessNegativeFlags(args)

	if !negated["splash"] {
		t.Error("expected splash to be negated")
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(result), result)
	}
	if result[0] != "-splash=false" {
		t.Errorf("expected -splash=false, got %q", result[0])
	}
	if result[1] != "-testnet" {
		t.Errorf("expected -testnet, got %q", result[1])
	}
}

func TestProcessNegativeFlags_NoTestnet(t *testing.T) {
	args := []string{"-notestnet", "-datadir=/path"}
	result, negated := ProcessNegativeFlags(args)

	if !negated["testnet"] {
		t.Error("expected testnet to be negated")
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(result), result)
	}
	if result[0] != "-testnet=false" {
		t.Errorf("expected -testnet=false, got %q", result[0])
	}
}

func TestProcessNegativeFlags_Node(t *testing.T) {
	// -node should NOT be treated as -no + de
	args := []string{"-node=127.0.0.1"}
	result, negated := ProcessNegativeFlags(args)

	if negated["de"] {
		t.Error("-node should not be treated as negation of 'de'")
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(result), result)
	}
	if result[0] != "-node=127.0.0.1" {
		t.Errorf("expected -node=127.0.0.1, got %q", result[0])
	}
}

func TestNormalizeAndProcessArgs(t *testing.T) {
	args := []string{"program", "--nosplash", "-testnet"}
	result, negated := NormalizeAndProcessArgs(args)

	if !negated["splash"] {
		t.Error("expected splash to be negated")
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(result), result)
	}
	if result[0] != "-splash=false" {
		t.Errorf("expected -splash=false, got %q", result[0])
	}
	if result[1] != "-testnet" {
		t.Errorf("expected -testnet, got %q", result[1])
	}
}

func TestIsHelpFlag(t *testing.T) {
	testCases := []struct {
		arg      string
		expected bool
	}{
		{"-?", true},
		{"-h", true},
		{"-help", true},
		{"--help", true},
		{"-H", true},
		{"-HELP", true},
		{"-version", false},
		{"-testnet", false},
	}

	for _, tc := range testCases {
		result := IsHelpFlag(tc.arg)
		if result != tc.expected {
			t.Errorf("IsHelpFlag(%q): expected %v, got %v", tc.arg, tc.expected, result)
		}
	}
}

func TestIsVersionFlag(t *testing.T) {
	testCases := []struct {
		arg      string
		expected bool
	}{
		{"-v", true},
		{"-V", true},
		{"-version", true},
		{"--version", true},
		{"-VERSION", true},
		{"-help", false},
		{"-testnet", false},
	}

	for _, tc := range testCases {
		result := IsVersionFlag(tc.arg)
		if result != tc.expected {
			t.Errorf("IsVersionFlag(%q): expected %v, got %v", tc.arg, tc.expected, result)
		}
	}
}

func TestHasHelpOrVersionFlag(t *testing.T) {
	testCases := []struct {
		args        []string
		expectHelp  bool
		expectVer   bool
	}{
		{[]string{"-testnet"}, false, false},
		{[]string{"-help"}, true, false},
		{[]string{"-version"}, false, true},
		{[]string{"-h", "-testnet"}, true, false},
		{[]string{"-testnet", "-V"}, false, true},
		{[]string{"-help", "-version"}, true, true},
	}

	for _, tc := range testCases {
		hasHelp, hasVer := HasHelpOrVersionFlag(tc.args)
		if hasHelp != tc.expectHelp {
			t.Errorf("HasHelpOrVersionFlag(%v): expected help=%v, got %v", tc.args, tc.expectHelp, hasHelp)
		}
		if hasVer != tc.expectVer {
			t.Errorf("HasHelpOrVersionFlag(%v): expected version=%v, got %v", tc.args, tc.expectVer, hasVer)
		}
	}
}
