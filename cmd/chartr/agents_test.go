package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListBakedAgentsPrintsSlugsOnePerLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baked-agents")
	body := "# baked at image build time\nclaude-code\ncodex\naider\ngemini-cli\nopencode\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out, errb bytes.Buffer
	if code := listBakedAgents(path, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	want := "claude-code\ncodex\naider\ngemini-cli\nopencode\n"
	if got := out.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestListBakedAgentsMissingFileIsEmptyAndZeroExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist")
	var out, errb bytes.Buffer
	code := listBakedAgents(path, &out, &errb)
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	if errb.String() != "" {
		t.Errorf("stderr = %q, want empty", errb.String())
	}
}

func TestListBakedAgentsBinaryGarbageIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baked-agents")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 'x', 0xff}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out, errb bytes.Buffer
	code := listBakedAgents(path, &out, &errb)
	if code == 0 {
		t.Fatalf("binary garbage was accepted (stdout %q)", out.String())
	}
	if !strings.Contains(errb.String(), "plain-text") {
		t.Errorf("stderr should name the manifest problem; got %q", errb.String())
	}
}

// The dispatcher itself: unknown subcommand emits usage and a non-zero exit,
// --help is zero. This is what `chartr agents --help` and `chartr --help`
// end up reaching.
func TestAgentsCmdDispatch(t *testing.T) {
	t.Run("no args prints usage and non-zero", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := agentsCmd(nil, &out, &errb); code == 0 {
			t.Errorf("no-args should be non-zero")
		}
		if !strings.Contains(errb.String(), "list-baked") {
			t.Errorf("usage should mention list-baked; got %q", errb.String())
		}
	})
	t.Run("--help is zero and mentions list-baked", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := agentsCmd([]string{"--help"}, &out, &errb); code != 0 {
			t.Errorf("--help = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "list-baked") {
			t.Errorf("--help should list list-baked; got %q", out.String())
		}
	})
	t.Run("unknown subcommand is non-zero", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := agentsCmd([]string{"nope"}, &out, &errb); code == 0 {
			t.Errorf("unknown subcommand should be non-zero")
		}
	})
}
