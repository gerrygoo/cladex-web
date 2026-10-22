package pdf

import (
	"context"
	"errors"
	"testing"
	"time"
)

// With every slot taken, Render waits for one rather than starting another typst, and
// gives up when the request does.
func TestRenderWaitsForASlot(t *testing.T) {
	for range cap(slots) {
		slots <- struct{}{}
	}
	defer func() {
		for range cap(slots) {
			<-slots
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Render(ctx, "= hola", time.Time{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the request's deadline", err)
	}
}
