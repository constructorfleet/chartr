package env

import "os"

// Resolve returns the first non-empty of flagVal, the environment variable
// named by envKey, and def — the flag > env > default precedence chartr's
// entrypoints apply so a container image can drive configuration purely
// through ENV while a flag on the command line still wins.
//
// An empty envKey skips the env lookup entirely, which is how a caller with
// no env fallback still reaches the same helper (and the same test coverage).
func Resolve(flagVal, envKey, def string) string {
	if flagVal != "" {
		return flagVal
	}
	if envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	return def
}
