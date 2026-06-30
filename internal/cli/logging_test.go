package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoggerFor_Verbose(t *testing.T) {
	var buf bytes.Buffer
	loggerFor(true, &buf).Debug("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("verbose logger should emit debug, got %q", buf.String())
	}
}

func TestLoggerFor_Quiet(t *testing.T) {
	var buf bytes.Buffer
	loggerFor(false, &buf).Debug("hello")
	if buf.Len() != 0 {
		t.Errorf("non-verbose logger should suppress debug, got %q", buf.String())
	}
}
