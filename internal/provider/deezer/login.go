package deezer

import (
	"context"
	"fmt"

	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/deezer/auth"
)

const (
	stepBrowser = "browser"
	stepARL     = "arl"
)

func (p *Provider) LoginSteps() []provider.LoginStep {
	return []provider.LoginStep{
		{
			ID:       stepBrowser,
			Kind:     provider.LoginStepBrowser,
			Prompt:   "Press Enter to open Deezer login in your browser",
			WaitText: "Complete Deezer login in the browser",
		},
		{
			ID:       stepARL,
			Kind:     provider.LoginStepInput,
			Prompt:   "Paste ARL or Cookie header, then press Enter",
			WaitText: "Checking ARL",
			Masked:   true,
		},
	}
}

func (p *Provider) Submit(ctx context.Context, stepID, input string) (string, error) {
	switch stepID {
	case stepBrowser:
		return auth.BrowserLogin(ctx, auth.BrowserLoginOptions{})
	case stepARL:
		return auth.NormalizeARL(input)
	default:
		return "", fmt.Errorf("unknown Deezer login step %q", stepID)
	}
}
