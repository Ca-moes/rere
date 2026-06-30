package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// RunKRR executes the krr binary with the given args, expecting `-f json` on
// stdout, and parses the result. stderr is surfaced on failure.
func RunKRR(ctx context.Context, bin string, args []string) ([]Target, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run krr (%s): %w: %s", bin, err, stderr.String())
	}
	return ParseKRR(&stdout)
}
