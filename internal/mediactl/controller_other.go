//go:build !darwin

package mediactl

import "github.com/sfate/amp-tui/internal/playback"

type unsupportedController struct{}

func newController() Controller                                           { return unsupportedController{} }
func (unsupportedController) MediaCommands() <-chan playback.MediaCommand { return nil }
func (unsupportedController) UpdateMediaState(playback.MediaState)        {}
func (unsupportedController) Close()                                      {}
