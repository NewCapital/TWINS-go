package cli

import (
	"os"
	"runtime"
	"testing"
)

func TestParseGUIArgs_Defaults(t *testing.T) {
	args := []string{"twins-gui"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit for default args")
	}
	if config.ShowSplash != true {
		t.Errorf("expected ShowSplash=true, got %v", config.ShowSplash)
	}
	if config.SplashSet != false {
		t.Errorf("expected SplashSet=false, got %v", config.SplashSet)
	}
	if config.DataDir != "" {
		t.Errorf("expected empty DataDir, got %q", config.DataDir)
	}
	if config.Testnet != false {
		t.Errorf("expected Testnet=false, got %v", config.Testnet)
	}
	if config.Network != "mainnet" {
		t.Errorf("expected Network=mainnet, got %q", config.Network)
	}
}

func TestParseGUIArgs_DataDir(t *testing.T) {
	tempDir := t.TempDir()
	testPath := tempDir + "/twins-data"
	args := []string{"twins-gui", "-datadir=" + testPath}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.DataDir != testPath {
		t.Errorf("expected DataDir=%s, got %q", testPath, config.DataDir)
	}
}

func TestParseGUIArgs_Testnet(t *testing.T) {
	args := []string{"twins-gui", "-testnet"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Testnet != true {
		t.Errorf("expected Testnet=true, got %v", config.Testnet)
	}
	if config.Network != "testnet" {
		t.Errorf("expected Network=testnet, got %q", config.Network)
	}
}

func TestParseGUIArgs_Regtest(t *testing.T) {
	args := []string{"twins-gui", "-regtest"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Regtest != true {
		t.Errorf("expected Regtest=true, got %v", config.Regtest)
	}
	if config.Network != "regtest" {
		t.Errorf("expected Network=regtest, got %q", config.Network)
	}
}

func TestParseGUIArgs_TestnetAndRegtest_Error(t *testing.T) {
	args := []string{"twins-gui", "-testnet", "-regtest"}
	_, _, err := ParseGUIArgs(args)

	if err == nil {
		t.Fatal("expected error for -testnet and -regtest together")
	}
}

func TestParseGUIArgs_Min(t *testing.T) {
	args := []string{"twins-gui", "-min"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.StartMinimized != true {
		t.Errorf("expected StartMinimized=true, got %v", config.StartMinimized)
	}
}

func TestParseGUIArgs_SplashExplicitTrue(t *testing.T) {
	args := []string{"twins-gui", "-splash=true"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.ShowSplash != true {
		t.Errorf("expected ShowSplash=true, got %v", config.ShowSplash)
	}
	if config.SplashSet != true {
		t.Errorf("expected SplashSet=true when -splash explicitly set, got %v", config.SplashSet)
	}
}

func TestParseGUIArgs_SplashExplicitFalse(t *testing.T) {
	args := []string{"twins-gui", "-splash=false"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.ShowSplash != false {
		t.Errorf("expected ShowSplash=false, got %v", config.ShowSplash)
	}
	if config.SplashSet != true {
		t.Errorf("expected SplashSet=true when -splash explicitly set, got %v", config.SplashSet)
	}
}

func TestParseGUIArgs_NoSplash(t *testing.T) {
	args := []string{"twins-gui", "-nosplash"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.ShowSplash != false {
		t.Errorf("expected ShowSplash=false with -nosplash, got %v", config.ShowSplash)
	}
	if config.SplashSet != true {
		t.Errorf("expected SplashSet=true with -nosplash, got %v", config.SplashSet)
	}
}

func TestParseGUIArgs_ChooseDataDir(t *testing.T) {
	args := []string{"twins-gui", "-choosedatadir"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.ChooseDataDir != true {
		t.Errorf("expected ChooseDataDir=true, got %v", config.ChooseDataDir)
	}
}

func TestParseGUIArgs_Lang(t *testing.T) {
	args := []string{"twins-gui", "-lang=es"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Language != "es" {
		t.Errorf("expected Language=es, got %q", config.Language)
	}
}

func TestParseGUIArgs_LangWithSpace(t *testing.T) {
	args := []string{"twins-gui", "-lang", "ru"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Language != "ru" {
		t.Errorf("expected Language=ru, got %q", config.Language)
	}
}

func TestParseGUIArgs_WindowTitle(t *testing.T) {
	args := []string{"twins-gui", "-windowtitle=MyWallet"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.WindowTitle != "MyWallet" {
		t.Errorf("expected WindowTitle=MyWallet, got %q", config.WindowTitle)
	}
}

func TestParseGUIArgs_CombinedFlags(t *testing.T) {
	tempDir := t.TempDir()
	testPath := tempDir + "/twins-data"
	args := []string{"twins-gui", "-testnet", "-min", "-nosplash", "-datadir=" + testPath, "-lang=uk"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Testnet != true {
		t.Errorf("expected Testnet=true, got %v", config.Testnet)
	}
	if config.StartMinimized != true {
		t.Errorf("expected StartMinimized=true, got %v", config.StartMinimized)
	}
	if config.ShowSplash != false {
		t.Errorf("expected ShowSplash=false, got %v", config.ShowSplash)
	}
	if config.DataDir != testPath {
		t.Errorf("expected DataDir=%s, got %q", testPath, config.DataDir)
	}
	if config.Language != "uk" {
		t.Errorf("expected Language=uk, got %q", config.Language)
	}
}

func TestParseGUIArgs_GNUStyle(t *testing.T) {
	args := []string{"twins-gui", "--testnet", "--min"}
	config, shouldExit, err := ParseGUIArgs(args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shouldExit {
		t.Fatal("should not exit")
	}
	if config.Testnet != true {
		t.Errorf("expected Testnet=true with GNU style, got %v", config.Testnet)
	}
	if config.StartMinimized != true {
		t.Errorf("expected StartMinimized=true with GNU style, got %v", config.StartMinimized)
	}
}

func TestExpandPath_Tilde(t *testing.T) {
	result, err := ExpandPath("~/test/path")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0] == '~' {
		t.Errorf("expected ~ to be expanded, got %q", result)
	}
	if len(result) < 10 {
		t.Errorf("path seems too short: %q", result)
	}
}

func TestExpandPath_Absolute(t *testing.T) {
	result, err := ExpandPath("/absolute/path")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/absolute/path" {
		t.Errorf("expected /absolute/path, got %q", result)
	}
}

func TestExpandPath_RejectsTraversal(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{"simple traversal", "../../etc/passwd"},
		{"deep traversal", "../../../../../../../etc/passwd"},
		{"single parent", "../secret"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExpandPath(tc.path)
			if err == nil {
				t.Errorf("expected error for path traversal attempt: %s", tc.path)
			}
		})
	}
}

func TestExpandPath_AcceptsValidPaths(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{"absolute unix path", "/home/user/twins-data"},
		{"home expansion", "~/twins-data"},
		{"relative path", "twins-data"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandPath(tc.path)
			if err != nil {
				t.Errorf("unexpected error for valid path %s: %v", tc.path, err)
			}
			if result == "" {
				t.Error("result should not be empty")
			}
		})
	}
}

func TestExpandPath_EmptyPath(t *testing.T) {
	_, err := ExpandPath("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestValidateDataDir_ExistingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	err := ValidateDataDir(tempDir)
	if err != nil {
		t.Errorf("unexpected error for existing directory: %v", err)
	}
}

func TestValidateDataDir_NonExistentWithValidParent(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := tempDir + "/newdir"
	err := ValidateDataDir(nonExistent)
	if err != nil {
		t.Errorf("unexpected error for creatable directory: %v", err)
	}
}

func TestValidateDataDir_NonExistentParent(t *testing.T) {
	err := ValidateDataDir("/nonexistent/parent/path/data")
	if err == nil {
		t.Error("expected error for non-existent parent directory")
	}
}

func TestValidateDataDir_FileNotDirectory(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := tempDir + "/file.txt"
	if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err := ValidateDataDir(tempFile)
	if err == nil {
		t.Error("expected error when path is a file, not directory")
	}
}

func TestGetDefaultGUIDataDir(t *testing.T) {
	dataDir := GetDefaultGUIDataDir()

	if dataDir == "" {
		t.Error("expected non-empty default data directory")
	}

	switch runtime.GOOS {
	case "windows":
		if !containsIgnoreCase(dataDir, "TWINS") {
			t.Errorf("Windows dataDir should contain TWINS: %q", dataDir)
		}
	case "darwin":
		if !containsIgnoreCase(dataDir, "TWINS") {
			t.Errorf("macOS dataDir should contain TWINS: %q", dataDir)
		}
	default:
		if !containsIgnoreCase(dataDir, ".twins") {
			t.Errorf("Linux dataDir should contain .twins: %q", dataDir)
		}
	}
}

func TestGUIConfig_ToMap(t *testing.T) {
	config := &GUIConfig{
		DataDir:        "/test/path",
		Testnet:        true,
		Network:        "testnet",
		StartMinimized: true,
		ShowSplash:     false,
		Language:       "uk",
	}

	m := config.ToMap()

	if m["dataDir"] != "/test/path" {
		t.Errorf("expected dataDir=/test/path, got %v", m["dataDir"])
	}
	if m["testnet"] != true {
		t.Errorf("expected testnet=true, got %v", m["testnet"])
	}
	if m["network"] != "testnet" {
		t.Errorf("expected network=testnet, got %v", m["network"])
	}
	if m["startMinimized"] != true {
		t.Errorf("expected startMinimized=true, got %v", m["startMinimized"])
	}
	if m["showSplash"] != false {
		t.Errorf("expected showSplash=false, got %v", m["showSplash"])
	}
	if m["language"] != "uk" {
		t.Errorf("expected language=uk, got %v", m["language"])
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	sLower := make([]byte, len(s))
	substrLower := make([]byte, len(substr))
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			sLower[i] = s[i] + 32
		} else {
			sLower[i] = s[i]
		}
	}
	for i := range substr {
		if substr[i] >= 'A' && substr[i] <= 'Z' {
			substrLower[i] = substr[i] + 32
		} else {
			substrLower[i] = substr[i]
		}
	}
	return contains(string(sLower), string(substrLower))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
