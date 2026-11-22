package tui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/plugins"
	"vex/internal/search"
)

const (
	defaultSearchLimit = 12
	maxRunnerUps       = 3
	maxGlancePills     = 3
)

// DefaultPinnedCategories provides the fallback 1-5 category bar when UserPrefs.PinnedCategories is empty.
var DefaultPinnedCategories = []string{
	"ports",
	"air-freight",
	"customs",
}

// Config configures the interactive TUI model.
type Config struct {
	AppName          string
	Edition          string
	Version          string
	PinnedCategories []string
	Search           *search.Service
	Store            *indexer.Store
	Runner           *plugins.Runner
}

// Model implements tea.Model for the Vex terminal interface.
type Model struct {
	cfg        Config
	categories []string // up to 4 pinned categories (slots 2..5; slot 1 is Always "All")

	query          string
	activeCategory string // "" means "All" (slot 1)
	pendingDigit   rune   // disambiguates single-key '1'..'5' category shortcut vs multi-digit number query
	prevCategory   string

	parsed         search.ParsedQuery
	results        []indexer.SearchResult
	promotedCardID string
	focusIndex     int    // 0 = Primary Hero Card, 1..N = Compact Runner-Ups
	activeTab      string // "base" or section.ModuleID

	statusMsg string
	statusErr bool
	width     int
	height    int
	quitting  bool
}

// NewModel constructs an initialized TUI Model and executes the initial search.
func NewModel(cfg Config) Model {
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = "Vex"
	}
	cats := normalizePinnedCategories(cfg.PinnedCategories)
	m := Model{
		cfg:        cfg,
		categories: cats,
		activeTab:  "base",
		width:      92,
		height:     28,
	}
	m.refreshSearch()
	return m
}

func normalizePinnedCategories(raw []string) []string {
	source := raw
	if len(source) == 0 {
		source = DefaultPinnedCategories
	}
	seen := make(map[string]bool, len(source))
	out := make([]string, 0, 4)
	for _, c := range source {
		clean := strings.TrimSpace(c)
		if clean == "" || strings.EqualFold(clean, "all") {
			continue
		}
		key := strings.ToLower(clean)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, clean)
		if len(out) == 4 {
			break
		}
	}
	return out
}

// Run launches the interactive Bubble Tea program in the terminal.
func Run(cfg Config, in io.Reader, out io.Writer) error {
	m := NewModel(cfg)
	var opts []tea.ProgramOption
	if in != nil {
		opts = append(opts, tea.WithInput(in))
	}
	if out != nil {
		opts = append(opts, tea.WithOutput(out))
	} else {
		opts = append(opts, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, opts...)
	_, err := p.Run()
	return err
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Query returns the current raw search input text.
func (m Model) Query() string {
	return m.query
}

// ActiveCategory returns the active category filter ("" for All).
func (m Model) ActiveCategory() string {
	return m.activeCategory
}

// ActiveTab returns the currently selected perspective tab on the Primary Hero Card ("base" or moduleId).
func (m Model) ActiveTab() string {
	return m.activeTab
}

// FocusIndex returns 0 when the Primary Hero Card is focused, or 1..N when a runner-up row is focused.
func (m Model) FocusIndex() int {
	return m.focusIndex
}

// HeroCard returns the active Primary Hero Card (if any results exist).
func (m Model) HeroCard() *models.SearchCard {
	hero, _ := m.heroAndRunnerUps()
	return hero
}

// RunnerUpCards returns up to 3 compact runner-up cards beneath the Primary Hero Card.
func (m Model) RunnerUpCards() []models.SearchCard {
	_, runners := m.heroAndRunnerUps()
	return runners
}

// StatusMessage returns the latest action execution banner text.
func (m Model) StatusMessage() string {
	return m.statusMsg
}

// Quitting reports whether the TUI has exited via staged Esc or Ctrl+C.
func (m Model) Quitting() bool {
	return m.quitting
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""
		m.statusErr = false

		keyStr := msg.String()
		switch keyStr {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "esc":
			m.pendingDigit = 0
			// Staged Esc: reset promoted runner-up -> clear category scope -> clear query -> quit
			if m.promotedCardID != "" {
				m.promotedCardID = ""
				m.focusIndex = 0
				m.syncHeroTab()
				return m, nil
			}
			if m.activeCategory != "" {
				m.activeCategory = ""
				m.promotedCardID = ""
				m.focusIndex = 0
				m.refreshSearch()
				return m, nil
			}
			if m.query != "" {
				m.query = ""
				m.promotedCardID = ""
				m.focusIndex = 0
				m.refreshSearch()
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "up", "ctrl+p":
			m.pendingDigit = 0
			if m.focusIndex > 0 {
				m.focusIndex--
			}
			return m, nil

		case "down", "ctrl+n":
			m.pendingDigit = 0
			_, runners := m.heroAndRunnerUps()
			if m.focusIndex < len(runners) {
				m.focusIndex++
			}
			return m, nil

		case "tab":
			m.pendingDigit = 0
			m.cyclePerspectiveTab(1)
			return m, nil

		case "shift+tab":
			m.pendingDigit = 0
			m.cyclePerspectiveTab(-1)
			return m, nil

		case "enter":
			m.pendingDigit = 0
			hero, runners := m.heroAndRunnerUps()
			if hero == nil {
				return m, nil
			}
			if m.focusIndex > 0 && m.focusIndex <= len(runners) {
				chosen := runners[m.focusIndex-1]
				m.promotedCardID = chosen.ID
				m.focusIndex = 0
				m.syncHeroTab()
				return m, nil
			}
			m.executeHeroAction(*hero)
			return m, nil

		case "backspace", "ctrl+h":
			m.pendingDigit = 0
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.promotedCardID = ""
				m.focusIndex = 0
				m.refreshSearch()
			}
			return m, nil

		case "ctrl+u":
			m.pendingDigit = 0
			if m.query != "" {
				m.query = ""
				m.promotedCardID = ""
				m.focusIndex = 0
				m.refreshSearch()
			}
			return m, nil

		case "alt+1", "f1":
			m.pendingDigit = 0
			m.selectCategorySlot(1)
			return m, nil
		case "alt+2", "f2":
			m.pendingDigit = 0
			m.selectCategorySlot(2)
			return m, nil
		case "alt+3", "f3":
			m.pendingDigit = 0
			m.selectCategorySlot(3)
			return m, nil
		case "alt+4", "f4":
			m.pendingDigit = 0
			m.selectCategorySlot(4)
			return m, nil
		case "alt+5", "f5":
			m.pendingDigit = 0
			m.selectCategorySlot(5)
			return m, nil
		}

		// Handle printable rune input (including 1-4 / 1-5 category shortcuts & query text)
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			runes := msg.Runes
			if msg.Type == tea.KeySpace {
				runes = []rune{' '}
			}
			if len(runes) == 1 {
				r := runes[0]
				// Disambiguate single-key category shortcut ('1'..'5') vs starting a numeric query ("100 kg to lb")
				if m.pendingDigit != 0 && (unicode.IsDigit(r) || r == '.') {
					m.activeCategory = m.prevCategory
					m.query = string([]rune{m.pendingDigit, r})
					m.pendingDigit = 0
					m.promotedCardID = ""
					m.focusIndex = 0
					m.refreshSearch()
					return m, nil
				}
				if (m.query == "" || m.focusIndex > 0) && r >= '1' && r <= '5' {
					slot := int(r - '0')
					if slot == 1 || slot-2 < len(m.categories) {
						m.prevCategory = m.activeCategory
						m.pendingDigit = r
						m.selectCategorySlot(slot)
						return m, nil
					}
				}
			}

			m.pendingDigit = 0
			m.query += string(runes)
			m.promotedCardID = ""
			m.focusIndex = 0
			m.refreshSearch()
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) selectCategorySlot(slot int) {
	if slot <= 1 {
		m.activeCategory = ""
	} else {
		idx := slot - 2
		if idx >= 0 && idx < len(m.categories) {
			target := m.categories[idx]
			if strings.EqualFold(m.activeCategory, target) {
				m.activeCategory = ""
			} else {
				m.activeCategory = target
			}
		}
	}
	m.promotedCardID = ""
	m.focusIndex = 0
	m.refreshSearch()
}

func (m *Model) effectiveQuery() string {
	q := strings.TrimSpace(m.query)
	if m.activeCategory == "" {
		return q
	}
	// Do not inject @collection if the user explicitly typed an @collection token
	if strings.Contains(q, "@") {
		return q
	}
	if q == "" {
		return "@" + m.activeCategory
	}
	return "@" + m.activeCategory + " " + q
}

func (m *Model) refreshSearch() {
	eff := m.effectiveQuery()
	m.parsed = search.Parse(eff)
	if m.cfg.Search == nil {
		m.results = nil
		m.activeTab = "base"
		return
	}
	resp, err := m.cfg.Search.Search(context.Background(), eff, defaultSearchLimit)
	if err != nil || resp == nil {
		m.results = nil
		m.activeTab = "base"
		return
	}
	m.results = resp.Results
	m.syncHeroTab()
}

func (m Model) heroAndRunnerUps() (*models.SearchCard, []models.SearchCard) {
	if len(m.results) == 0 {
		return nil, nil
	}
	heroIdx := 0
	if m.promotedCardID != "" {
		for i, r := range m.results {
			if r.Card.ID == m.promotedCardID {
				heroIdx = i
				break
			}
		}
	}
	hero := m.results[heroIdx].Card
	runners := make([]models.SearchCard, 0, maxRunnerUps)
	for i, r := range m.results {
		if i == heroIdx {
			continue
		}
		runners = append(runners, r.Card)
		if len(runners) == maxRunnerUps {
			break
		}
	}
	return &hero, runners
}

func (m *Model) syncHeroTab() {
	hero, _ := m.heroAndRunnerUps()
	if hero == nil || len(hero.Sections) == 0 {
		m.activeTab = "base"
		return
	}
	m.activeTab = autoSelectPerspectiveTab(*hero, m.query)
}

func autoSelectPerspectiveTab(card models.SearchCard, rawQuery string) string {
	if len(card.Sections) == 0 {
		return "base"
	}
	q := strings.ToLower(strings.TrimSpace(rawQuery))
	if q == "" {
		return "base"
	}

	titleLower := strings.ToLower(card.Title)
	var sectionTokens []string
	for _, tok := range strings.Fields(q) {
		if strings.HasPrefix(tok, "@") || strings.HasPrefix(tok, "#") || strings.HasPrefix(tok, "/") {
			continue
		}
		if len(tok) >= 2 && !strings.Contains(titleLower, tok) {
			sectionTokens = append(sectionTokens, tok)
		}
	}
	if len(sectionTokens) == 0 {
		return "base"
	}

	for _, sec := range card.Sections {
		for _, cand := range sectionTokens {
			if strings.Contains(strings.ToLower(sec.ModuleID), cand) ||
				strings.Contains(strings.ToLower(sec.ModuleName), cand) ||
				strings.Contains(strings.ToLower(sec.Badge), cand) ||
				strings.Contains(strings.ToLower(sec.Title), cand) ||
				strings.Contains(strings.ToLower(sec.Notes), cand) {
				return sec.ModuleID
			}
			for k, v := range sec.Fields {
				if strings.Contains(strings.ToLower(k), cand) ||
					strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), cand) {
					return sec.ModuleID
				}
			}
		}
	}
	return "base"
}

func (m *Model) cyclePerspectiveTab(delta int) {
	hero, _ := m.heroAndRunnerUps()
	if hero == nil || len(hero.Sections) == 0 {
		m.activeTab = "base"
		return
	}
	tabs := make([]string, 0, 1+len(hero.Sections))
	tabs = append(tabs, "base")
	for _, sec := range hero.Sections {
		tabs = append(tabs, sec.ModuleID)
	}
	cur := 0
	for i, t := range tabs {
		if t == m.activeTab {
			cur = i
			break
		}
	}
	next := (cur + delta) % len(tabs)
	if next < 0 {
		next += len(tabs)
	}
	m.activeTab = tabs[next]
}

func (m *Model) executeHeroAction(hero models.SearchCard) {
	var act *models.CardAction
	if m.activeTab != "base" {
		for _, sec := range hero.Sections {
			if sec.ModuleID == m.activeTab && len(sec.Actions) > 0 {
				a := sec.Actions[0]
				act = &a
				break
			}
		}
	}
	if act == nil && len(hero.Actions) > 0 {
		a := hero.Actions[0]
		act = &a
	}

	if m.cfg.Runner != nil && act != nil {
		res := m.cfg.Runner.Execute(context.Background(), hero, act.ID, m.parsed, nil)
		if res.Success {
			m.statusMsg = fmt.Sprintf("✓ %s (%s)", firstNonEmpty(res.Output, act.Label), res.ActionType)
			m.statusErr = false
		} else {
			m.statusMsg = fmt.Sprintf("✗ %s", firstNonEmpty(res.Error, "Action failed"))
			m.statusErr = true
		}
		return
	}

	if act != nil {
		m.statusMsg = fmt.Sprintf("✓ %s: %s", act.Label, act.Target)
		m.statusErr = false
		return
	}
	m.statusMsg = fmt.Sprintf("✓ Selected %s", hero.Title)
	m.statusErr = false
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	activePillStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63")).Padding(0, 1)
	inactivePillStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Background(lipgloss.Color("237")).Padding(0, 1)
	glanceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Background(lipgloss.Color("236")).Padding(0, 1)
	localOnlyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Background(lipgloss.Color("236")).Padding(0, 1)

	heroBorderColor := lipgloss.Color("63")
	if m.focusIndex > 0 {
		heroBorderColor = lipgloss.Color("240")
	}
	heroBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(heroBorderColor).
		Padding(0, 1).
		Width(maxInt(64, m.width-4))

	var b strings.Builder

	// 1. Oval Search Bar
	prompt := m.query
	if prompt == "" {
		prompt = mutedStyle.Render("Search cards, ports, 100 kg to lb, 120x80x100 cm...")
	} else {
		prompt = headerStyle.Render(m.query) + "█"
	}
	fmt.Fprintf(&b, " ⌕ %s   %s\n", prompt, mutedStyle.Render("["+m.cfg.AppName+"]"))

	// 2. Category Modifier Bar (1 All | 2..5 PinnedCategories)
	var catPills []string
	if m.activeCategory == "" {
		catPills = append(catPills, activePillStyle.Render("1 All"))
	} else {
		catPills = append(catPills, inactivePillStyle.Render("1 All"))
	}
	for i, cat := range m.categories {
		label := fmt.Sprintf("%d %s", i+2, formatCategoryLabel(cat))
		if strings.EqualFold(m.activeCategory, cat) {
			catPills = append(catPills, activePillStyle.Render(label))
		} else {
			catPills = append(catPills, inactivePillStyle.Render(label))
		}
	}
	fmt.Fprintf(&b, " %s\n\n", strings.Join(catPills, " "))

	// 3. Primary Hero Card + Compact Runner-Ups
	hero, runners := m.heroAndRunnerUps()
	if hero == nil {
		b.WriteString(mutedStyle.Render("   No matching cards."))
		b.WriteByte('\n')
	} else {
		b.WriteString(heroBoxStyle.Render(m.renderHeroContent(*hero, headerStyle, mutedStyle, activePillStyle, inactivePillStyle, glanceStyle, localOnlyStyle)))
		b.WriteByte('\n')

		if len(runners) > 0 {
			dividerWidth := maxInt(48, m.width-6)
			fmt.Fprintf(&b, " %s\n", mutedStyle.Render(strings.Repeat("─", dividerWidth)))
			for i, r := range runners {
				focused := m.focusIndex == i+1
				prefix := "   "
				titleRendered := r.Title
				if focused {
					prefix = " ▸ "
					titleRendered = headerStyle.Render(r.Title)
				}
				var pills []string
				for j, p := range r.GlancePills {
					if j >= maxGlancePills {
						break
					}
					pills = append(pills, glanceStyle.Render(p))
				}
				if len(pills) == 0 && r.Collection != "" {
					pills = append(pills, inactivePillStyle.Render("@"+r.Collection))
				}
				sub := ""
				if r.Subtitle != "" {
					sub = " " + mutedStyle.Render("— "+truncateText(r.Subtitle, 42))
				}
				pillBlock := ""
				if len(pills) > 0 {
					pillBlock = "  " + strings.Join(pills, " ")
				}
				fmt.Fprintf(&b, "%s%s%s%s\n", prefix, titleRendered, sub, pillBlock)
			}
		}
	}

	// 4. Status & Keybinding Footer
	b.WriteByte('\n')
	if m.statusMsg != "" {
		fmt.Fprintf(&b, " %s\n", m.statusMsg)
	}
	b.WriteString(mutedStyle.Render(" [↑/↓] Focus  [Enter] Promote/Run  [Tab] Perspectives  [1-4/Alt+1-4] Category  [Esc] Back/Quit"))
	b.WriteByte('\n')
	return b.String()
}

func (m Model) renderHeroContent(
	hero models.SearchCard,
	headerStyle, mutedStyle, activePillStyle, inactivePillStyle, glanceStyle, localOnlyStyle lipgloss.Style,
) string {
	var hb strings.Builder

	focusMarker := ""
	if m.focusIndex == 0 {
		focusMarker = "▸ "
	}
	var rightBadges []string
	for i, gp := range hero.GlancePills {
		if i >= maxGlancePills {
			break
		}
		rightBadges = append(rightBadges, glanceStyle.Render(gp))
	}
	isLocal := hero.LocalOnly || (hero.Provenance != nil && hero.Provenance.LocalOnly)
	var activeSec *models.CardSection
	for i := range hero.Sections {
		if hero.Sections[i].ModuleID == m.activeTab {
			activeSec = &hero.Sections[i]
			if activeSec.LocalOnly || (activeSec.Provenance != nil && activeSec.Provenance.LocalOnly) {
				isLocal = true
			}
			break
		}
	}
	if isLocal {
		rightBadges = append(rightBadges, localOnlyStyle.Render("Local Only"))
	}

	titleLine := focusMarker + headerStyle.Render(hero.Title)
	if hero.Subtitle != "" {
		titleLine += " " + mutedStyle.Render("· "+hero.Subtitle)
	}
	if len(rightBadges) > 0 {
		titleLine += "  " + strings.Join(rightBadges, " ")
	}
	hb.WriteString(titleLine)
	hb.WriteByte('\n')

	// Single-tab perspective bar when module overlays are attached
	if len(hero.Sections) > 0 {
		var tabs []string
		if m.activeTab == "base" {
			tabs = append(tabs, activePillStyle.Render("Overview"))
		} else {
			tabs = append(tabs, inactivePillStyle.Render("Overview"))
		}
		for _, sec := range hero.Sections {
			label := firstNonEmpty(sec.Badge, sec.ModuleName, sec.Title)
			if m.activeTab == sec.ModuleID {
				tabs = append(tabs, activePillStyle.Render(label))
			} else {
				tabs = append(tabs, inactivePillStyle.Render(label))
			}
		}
		hb.WriteString(strings.Join(tabs, " "))
		hb.WriteByte('\n')
	}

	// Body: either the active module perspective section or base Overview
	if activeSec != nil {
		if activeSec.Title != "" {
			hb.WriteString(headerStyle.Render(activeSec.Title))
			hb.WriteByte('\n')
		}
		if activeSec.Notes != "" {
			hb.WriteString(activeSec.Notes)
			hb.WriteByte('\n')
		}
		renderSortedFields(&hb, activeSec.Fields, mutedStyle)
	} else {
		if hero.Description != "" {
			hb.WriteString(hero.Description)
			hb.WriteByte('\n')
		}
		renderSortedFields(&hb, hero.Data, mutedStyle)
	}

	// Actions & Provenance footer
	actions := hero.Actions
	if activeSec != nil && len(activeSec.Actions) > 0 {
		actions = activeSec.Actions
	}
	if len(actions) > 0 {
		hb.WriteString(mutedStyle.Render(fmt.Sprintf("[Enter] %s (%s)", actions[0].Label, actions[0].Type)))
		hb.WriteByte('\n')
	}

	prov := hero.Provenance
	if activeSec != nil && activeSec.Provenance != nil {
		prov = activeSec.Provenance
	}
	if prov != nil && prov.SourceName != "" {
		footer := "Source: " + prov.SourceName
		if prov.Authority != "" {
			footer += " (" + prov.Authority + ")"
		}
		if prov.VerifiedAt != "" {
			footer += " · Verified " + prov.VerifiedAt
		}
		hb.WriteString(mutedStyle.Render(footer))
	}

	return strings.TrimRight(hb.String(), "\n")
}

func renderSortedFields(b *strings.Builder, fields map[string]any, mutedStyle lipgloss.Style) {
	if len(fields) == 0 {
		return
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	maxFields := 6
	if len(keys) < maxFields {
		maxFields = len(keys)
	}
	for i := 0; i < maxFields; i++ {
		k := keys[i]
		v := fields[k]
		if v == nil || fmt.Sprintf("%v", v) == "" {
			continue
		}
		fmt.Fprintf(b, "%s %v\n", mutedStyle.Render(k+":"), v)
	}
}

func formatCategoryLabel(col string) string {
	switch strings.ToLower(strings.TrimSpace(col)) {
	case "ports", "geographies":
		return "Geographies"
	case "air-freight", "air":
		return "Air"
	case "ocean-freight", "ocean":
		return "Ocean"
	case "customs":
		return "Customs"
	case "global-tech":
		return "Global Tech"
	case "github-catalog":
		return "GitHub"
	default:
		parts := strings.Split(strings.ReplaceAll(col, "_", "-"), "-")
		for i, p := range parts {
			if len(p) > 0 {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
		return strings.Join(parts, " ")
	}
}

func truncateText(s string, maxLen int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxLen || maxLen <= 1 {
		return string(runes)
	}
	return string(runes[:maxLen-1]) + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
