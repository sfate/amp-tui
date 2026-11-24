package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/provider"
)

const (
	settingsNavLabel   = "Settings"
	searchCollectionID = "__search__"
)

// catalogInfo outlives a rejected session, so the login screen keeps its texts.
type catalogInfo struct {
	name     string
	sections []provider.Section
	auth     provider.Authenticator
}

func catalogInfoOf(p provider.Provider) catalogInfo {
	if p == nil {
		return catalogInfo{}
	}
	authn, _ := p.(provider.Authenticator)
	return catalogInfo{name: p.Name(), sections: p.Sections(), auth: authn}
}

// collectionID keeps section IDs apart from provider playlist IDs.
func collectionID(sectionID string) string {
	return "__" + sectionID + "__"
}

func (m *Model) setCatalog(info catalogInfo) {
	m.catalog = info
	items := make([]string, 0, len(info.sections)+1)
	m.app.FlowCollectionID = ""
	for _, sec := range info.sections {
		items = append(items, sec.Label)
		if sec.Kind == provider.SectionRadio && m.app.FlowCollectionID == "" {
			m.app.FlowCollectionID = collectionID(sec.ID)
		}
	}
	m.app.NavItems = append(items, settingsNavLabel)
	if derefOrZero(m.app.NavState.Selected()) >= len(m.app.NavItems) {
		m.app.NavState.Select(intPtr(len(m.app.NavItems) - 1))
	}
	m.syncSettingsRows()
}

func (m Model) sectionByCollection(id string) (provider.Section, bool) {
	for _, sec := range m.catalog.sections {
		if collectionID(sec.ID) == id {
			return sec, true
		}
	}
	return provider.Section{}, false
}

func (m Model) currentSectionKind() (provider.SectionKind, bool) {
	if m.app.CurrentPlaylistID == nil {
		return 0, false
	}
	sec, ok := m.sectionByCollection(*m.app.CurrentPlaylistID)
	return sec.Kind, ok
}

func (m Model) viewingFavorites() bool {
	kind, ok := m.currentSectionKind()
	return ok && kind == provider.SectionFavorites
}

func (m Model) viewingRadio() bool {
	kind, ok := m.currentSectionKind()
	return ok && kind == provider.SectionRadio
}

func (m Model) collectionTitle(id string) string {
	if sec, ok := m.sectionByCollection(id); ok {
		return sec.Label
	}
	switch id {
	case searchCollectionID:
		return "Search"
	case "":
		return "Browse"
	default:
		return id
	}
}

// fromReplacedProvider is true for a message that an earlier provider sent before a switch.
func (m Model) fromReplacedProvider(source provider.ID) bool {
	return source != "" && (m.provider == nil || m.provider.ID() != source)
}

func (m Model) providerName() string {
	if m.catalog.name == "" {
		return "music provider"
	}
	return m.catalog.name
}

func (m Model) loadingStatus() string {
	return fmt.Sprintf("Loading %s library...", m.providerName())
}

// loadInitialSection opens the first section once the library is bootstrapped.
func (m *Model) loadInitialSection() tea.Cmd {
	if len(m.catalog.sections) == 0 {
		m.ready = true
		m.showNotice()
		return nil
	}
	return loadSectionCmd(m.provider, m.catalog.sections[0], false)
}

func (m Model) isInitialSection(id string) bool {
	return len(m.catalog.sections) > 0 && id == collectionID(m.catalog.sections[0].ID)
}

// loadMoreFlow extends the flow queue when the provider supports endless radio.
func (m *Model) loadMoreFlow(autoplay bool) tea.Cmd {
	if !m.app.ShouldLoadMoreFlow() {
		return nil
	}
	radio, ok := m.provider.(provider.InfiniteRadio)
	sec, found := m.sectionByCollection(m.app.FlowCollectionID)
	if !ok || !found || sec.Kind != provider.SectionRadio {
		return nil
	}
	m.app.FlowLoadingMore = true
	m.app.StatusMessage = fmt.Sprintf("Loading more %s...", sec.Label)
	return loadRadioPageCmd(m.provider.ID(), radio, sec, m.app.FlowNextIndex, autoplay)
}
