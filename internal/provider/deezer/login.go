package deezer

import (
	"context"
	"strings"

	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/deezer/auth"
)

func (p *Provider) LoginPrompt() provider.LoginPrompt {
	return provider.LoginPrompt{
		ConfirmText:     "Press Enter to open Deezer login in your browser",
		BrowserWaitText: "Complete Deezer login in the browser",
		PasteHint:       "Paste ARL or Cookie header, then press Enter",
		SecretName:      "ARL or Cookie header",
	}
}

func (p *Provider) NormalizeSecret(input string) (string, error) {
	return auth.NormalizeARL(input)
}

// MaskSecret also gets raw login input, so a pasted cookie header must not leak.
func (p *Provider) MaskSecret(secret string) string {
	if strings.Contains(secret, "arl=") {
		return "Cookie header pasted"
	}
	return auth.MaskARL(secret)
}

func (p *Provider) BrowserLogin(ctx context.Context) (string, error) {
	return auth.BrowserLogin(ctx, auth.BrowserLoginOptions{})
}
