package cli

import (
	"runtime"
	"strings"
)

// NormalizeArgs converts command-line arguments to standard Bitcoin-style format.
// It handles:
//   - Windows /arg style: /testnet → -testnet
//   - GNU --arg style: --testnet → -testnet
//   - -noX pattern: -nosplash → -splash=false
//
// This function should be called before passing args to any flag parser
// to ensure consistent behavior across platforms and compatibility with
// the legacy C++ implementation.
func NormalizeArgs(args []string) []string {
	if len(args) <= 1 {
		return []string{}
	}

	result := make([]string, 0, len(args)-1)
	for _, arg := range args[1:] { // Skip program name
		normalized := normalizeArg(arg)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

// normalizeArg normalizes a single argument to Bitcoin-style format
func normalizeArg(arg string) string {
	// Convert Windows /arg to -arg (only on Windows)
	if runtime.GOOS == "windows" && strings.HasPrefix(arg, "/") && len(arg) > 1 {
		arg = "-" + strings.ToLower(arg[1:])
	}

	// Convert GNU --arg to -arg
	if strings.HasPrefix(arg, "--") && len(arg) > 2 {
		arg = arg[1:] // Strip one dash
	}

	return arg
}

// ProcessNegativeFlags handles -noX patterns (e.g., -nosplash → -splash=false)
// Returns:
//   - processedArgs: arguments with -noX patterns converted
//   - negatedFlags: map of flag names that were negated (e.g., "splash" → true)
//
// This is separated from NormalizeArgs because -noX handling needs to happen
// after normalization but before flag parsing, and callers may need to know
// which flags were explicitly negated.
func ProcessNegativeFlags(args []string) (processedArgs []string, negatedFlags map[string]bool) {
	processedArgs = make([]string, 0, len(args))
	negatedFlags = make(map[string]bool)

	for _, arg := range args {
		lowerArg := strings.ToLower(arg)

		// Handle -noX patterns
		if strings.HasPrefix(lowerArg, "-no") && len(arg) > 3 {
			// Extract flag name after -no (may include =value)
			flagPart := arg[3:] // Keep original case for flag name
			lowerFlagPart := lowerArg[3:]

			// Extract base flag name (before =) for special case checking
			baseFlagName := lowerFlagPart
			if idx := strings.Index(lowerFlagPart, "="); idx != -1 {
				baseFlagName = lowerFlagPart[:idx]
			}

			// Special cases that are not negations:
			// -node, -nodes, -nonce, etc. - words starting with "no" prefix
			switch baseFlagName {
			case "de", "des", "nce", "tify", "tification": // -node, -nodes, -nonce, -notify, -notification
				processedArgs = append(processedArgs, arg)
				continue
			}

			// Convert -noX to -X=false
			negatedFlags[baseFlagName] = true
			processedArgs = append(processedArgs, "-"+flagPart+"=false")
			continue
		}

		processedArgs = append(processedArgs, arg)
	}

	return processedArgs, negatedFlags
}

// NormalizeAndProcessArgs combines NormalizeArgs and ProcessNegativeFlags
// for convenience. Returns normalized args with -noX patterns converted.
func NormalizeAndProcessArgs(args []string) ([]string, map[string]bool) {
	normalized := NormalizeArgs(args)
	return ProcessNegativeFlags(normalized)
}

// IsHelpFlag checks if the argument is a help flag (-?, -h, -help, --help)
func IsHelpFlag(arg string) bool {
	lower := strings.ToLower(arg)
	switch lower {
	case "-?", "-h", "-help", "--help", "/h", "/help", "/?":
		return true
	}
	return false
}

// IsVersionFlag checks if the argument is a version flag (-v, -V, -version, --version)
func IsVersionFlag(arg string) bool {
	lower := strings.ToLower(arg)
	switch lower {
	case "-v", "-version", "--version", "/v", "/version":
		return true
	}
	return false
}

// HasHelpOrVersionFlag scans args for help or version flags
// Returns (hasHelp, hasVersion)
func HasHelpOrVersionFlag(args []string) (bool, bool) {
	hasHelp := false
	hasVersion := false

	for _, arg := range args {
		if IsHelpFlag(arg) {
			hasHelp = true
		}
		if IsVersionFlag(arg) {
			hasVersion = true
		}
	}

	return hasHelp, hasVersion
}
