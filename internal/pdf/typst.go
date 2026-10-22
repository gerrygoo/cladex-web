// Package pdf renders Typst source to PDF bytes via the typst CLI, run as a subprocess.
// Typst was chosen over a headless-Chromium renderer specifically for its memory
// footprint — see docs/PLAN.md.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// slots bounds how many typst processes run at once. Each one takes tens of MB, and the
// NAS shares its memory with other services, so a burst of PDF requests queues here
// instead of spawning a process per request.
var slots = make(chan struct{}, 2)

// Render compiles Typst source into PDF bytes by piping it through `typst compile`.
// created is embedded as the PDF's creation date. Typst otherwise stamps the current
// time into the metadata, so pinning it is what makes the same source (and the same
// Typst version) render to byte-identical output. A zero created means "now".
func Render(ctx context.Context, source string, created time.Time) ([]byte, error) {
	if created.IsZero() {
		created = time.Now()
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("pdf: waiting to render: %w", ctx.Err())
	}
	cmd := exec.CommandContext(ctx, "typst", "compile", "--format", "pdf",
		"--creation-timestamp", strconv.FormatInt(created.Unix(), 10), "-", "-")
	cmd.Stdin = bytes.NewReader([]byte(source))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdf: typst compile: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
