// Package pdf renders Typst source to PDF bytes via the typst CLI, run as a subprocess.
// Typst was chosen over a headless-Chromium renderer specifically for its memory
// footprint — see docs/PLAN.md.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Render compiles Typst source into PDF bytes by piping it through `typst compile`.
func Render(ctx context.Context, source string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "typst", "compile", "--format", "pdf", "-", "-")
	cmd.Stdin = bytes.NewReader([]byte(source))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdf: typst compile: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
