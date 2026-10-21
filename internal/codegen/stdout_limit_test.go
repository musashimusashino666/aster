package codegen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/musashimusashino666/aster/internal/limits"
)

func TestNativeRuntimeEnforcesCumulativeStdoutLimit(t *testing.T) {
	original := "const asterMaxStdoutBytes = " + strconv.Itoa(limits.MaxStdoutBytes)
	runtime := strings.Replace(nativePreamble, original, "const asterMaxStdoutBytes = 8", 1)
	if runtime == nativePreamble {
		t.Fatalf("runtime preamble does not contain the shared stdout limit %q", original)
	}

	source := runtime + `

func main() {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintln(os.Stderr, recovered)
			os.Exit(1)
		}
	}()
	asterPrint("12345", 1, 1)
	asterPrint("abc", 1, 1)
	asterPrint("must not be written", 1, 1)
}
`
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected native runtime to reject output beyond the configured budget")
	}
	if got, want := stdout.String(), "12345\n"; got != want {
		t.Fatalf("stdout = %q, want %q; rejected print must not be partially written", got, want)
	}
	if !strings.Contains(stderr.String(), "stdout byte limit exceeded (maximum 8 bytes)") {
		t.Fatalf("stderr does not include cumulative stdout limit diagnostic: %q", stderr.String())
	}
}
