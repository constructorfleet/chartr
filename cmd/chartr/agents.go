package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// bakedAgentsPath is the location the Docker image writes its manifest of
// baked-in agent slugs to (D9 / ticket 07 / ticket 08). Overridable so tests
// don't touch /etc/chartr.
var bakedAgentsPath = "/etc/chartr/baked-agents"

// agentsUsage is what `chartr agents --help` prints and what an unknown
// `chartr agents <sub>` falls back to. One line per subcommand keeps it
// grep-friendly and short.
const agentsUsage = `chartr agents <subcommand>

Subcommands:
  list-baked   Print the slugs of agents baked into this image, one per line.
`

// agentsCmd dispatches `chartr agents ...`. It returns the exit code so
// main can hand it straight to os.Exit; stdout and stderr are the caller's.
func agentsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, agentsUsage)
		return 2
	}
	switch args[0] {
	case "list-baked":
		return listBakedAgents(bakedAgentsPath, stdout, stderr)
	case "--help", "-h", "help":
		fmt.Fprint(stdout, agentsUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "chartr agents: unknown subcommand %q\n\n%s", args[0], agentsUsage)
		return 2
	}
}

// listBakedAgents prints the slugs in path, one per line, to stdout.
//
// A missing file is not an error: non-container invocations never write it,
// and the operator would rather see empty output than a spurious failure.
// A file that is not plain text (a NUL byte or any other non-printable
// control other than tab/CR/LF) IS an error — that shape almost always
// means the wrong file, and printing binary garbage as slugs would be
// worse than nothing.
func listBakedAgents(path string, stdout, stderr io.Writer) int {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0
		}
		fmt.Fprintf(stderr, "chartr agents list-baked: %v\n", err)
		return 1
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		fmt.Fprintf(stderr, "chartr agents list-baked: %v\n", err)
		return 1
	}
	if isBinary(data) {
		fmt.Fprintf(stderr, "chartr agents list-baked: %s is not a plain-text manifest\n", path)
		return 1
	}

	w := bufio.NewWriter(stdout)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fmt.Fprintln(w, line)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(stderr, "chartr agents list-baked: %v\n", err)
		return 1
	}
	_ = w.Flush()
	return 0
}

// isBinary reports whether b contains a byte that would not appear in a
// plain-text manifest — a NUL or any control code other than the ordinary
// whitespace ones (tab, LF, CR).
func isBinary(b []byte) bool {
	for _, c := range b {
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}
