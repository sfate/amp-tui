// Package doctor checks the runtime dependencies of amp-tui.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Check is one runtime dependency check.
type Check struct {
	Name string
	Hint string
	Run  func(ctx context.Context) error
}

// ErrFailed reports that one or more checks failed.
var ErrFailed = errors.New("one or more dependency checks failed")

// Run executes the checks, writes one line per check to out, and returns ErrFailed if a check fails.
func Run(ctx context.Context, out io.Writer, checks []Check) error {
	failed := false
	for _, check := range checks {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := check.Run(checkCtx)
		cancel()
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(out, "FAIL  %s: %v\n", check.Name, err)
			if check.Hint != "" {
				_, _ = fmt.Fprintf(out, "      %s\n", check.Hint)
			}
			continue
		}
		_, _ = fmt.Fprintf(out, "ok    %s\n", check.Name)
	}
	if len(checks) == 0 {
		_, _ = fmt.Fprintln(out, "ok    no runtime dependencies on this platform")
	}
	if failed {
		return ErrFailed
	}
	return nil
}

func commandSucceeds(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w: %s", err, output)
		}
		return err
	}
	return nil
}
