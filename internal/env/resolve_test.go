package env_test

import (
	"testing"

	"github.com/rengwu/chartr/internal/env"
)

// The four precedence cases that make Resolve worth having, table-driven
// because the interesting behaviour is the ordering, not any one case.
func TestResolvePrecedence(t *testing.T) {
	const key = "CHARTR_TEST_RESOLVE"
	for _, tc := range []struct {
		name   string
		flag   string
		envVal string
		def    string
		want   string
	}{
		{
			name:   "flag beats env and default",
			flag:   "flag-value",
			envVal: "env-value",
			def:    "default-value",
			want:   "flag-value",
		},
		{
			name:   "env fills in when the flag is empty",
			flag:   "",
			envVal: "env-value",
			def:    "default-value",
			want:   "env-value",
		},
		{
			name:   "default when neither flag nor env is set",
			flag:   "",
			envVal: "",
			def:    "default-value",
			want:   "default-value",
		},
		{
			name:   "flag beats an unset env",
			flag:   "flag-value",
			envVal: "",
			def:    "default-value",
			want:   "flag-value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(key, tc.envVal)
			got := env.Resolve(tc.flag, key, tc.def)
			if got != tc.want {
				t.Errorf("Resolve(%q, %q=%q, %q) = %q, want %q", tc.flag, key, tc.envVal, tc.def, got, tc.want)
			}
		})
	}
}

// An empty envKey lets a caller reuse Resolve where there is no env fallback
// at all — same helper, no accidental read of a shared variable.
func TestResolveEmptyEnvKey(t *testing.T) {
	if got := env.Resolve("", "", "def"); got != "def" {
		t.Errorf("Resolve with empty envKey and empty flag = %q, want %q", got, "def")
	}
	if got := env.Resolve("flag", "", "def"); got != "flag" {
		t.Errorf("Resolve with empty envKey and set flag = %q, want %q", got, "flag")
	}
}
