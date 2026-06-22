package version

import "testing"

func TestStringIncludesDaemonName(t *testing.T) {
	if got := String(); got != "unumd 0.0.0-dev" {
		t.Fatalf("String() = %q", got)
	}
}
