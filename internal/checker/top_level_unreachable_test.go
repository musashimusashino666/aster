package checker

import (
	"strings"
	"testing"
)

func TestRejectsUnreachableTopLevelStatementAfterInfiniteLoop(t *testing.T) {
	err := checkSource(t, "while true { }\nprint(1);\n")
	if err == nil || !strings.Contains(err.Error(), "unreachable statement") {
		t.Fatalf("expected top-level unreachable statement error, got %v", err)
	}
}

func TestAcceptsTopLevelStatementsBeforeInfiniteLoop(t *testing.T) {
	if err := checkSource(t, "print(1);\nwhile true { }\n"); err != nil {
		t.Fatalf("expected statements before a non-fallthrough loop to be accepted, got %v", err)
	}
}
