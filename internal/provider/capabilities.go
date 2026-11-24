package provider

// Capabilities says which optional functions a provider supports.
// It is derived from the interfaces and sections, so a provider cannot declare a function it does not implement.
type Capabilities struct {
	Playback      bool
	QualityLadder bool
	Login         bool
	Radio         bool
	Favorites     bool
}

// CapabilitiesOf gives the zero value for a nil provider.
func CapabilitiesOf(p Provider) Capabilities {
	if p == nil {
		return Capabilities{}
	}
	var c Capabilities
	if streams, ok := p.(StreamSource); ok {
		c.Playback = true
		c.QualityLadder = len(streams.Qualities()) > 1
	}
	_, c.Login = p.(Authenticator)
	_, pager := p.(InfiniteRadio)
	for _, sec := range p.Sections() {
		switch sec.Kind {
		case SectionRadio:
			c.Radio = pager
		case SectionFavorites:
			c.Favorites = true
		}
	}
	return c
}
