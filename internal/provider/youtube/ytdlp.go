package youtube

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	if binary == "" {
		return nil, errors.New("yt-dlp executable was not found on PATH")
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.WaitDelay = 2_000_000_000
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	return output, nil
}
