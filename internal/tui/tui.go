package tui

import (
	"context"
	"os"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// styles holds every theme-derived lipgloss style. Each Model owns its own
// instance (built by newStyles), so concurrent SSH sessions can run different
// themes without racing on shared package state. Never mutate after creation.
type styles struct {
	subtleStyle  lipgloss.Style
	specialStyle lipgloss.Style
	warnStyle    lipgloss.Style
	staleStyle   lipgloss.Style
	dangerStyle  lipgloss.Style
	titleStyle   lipgloss.Style
	activeTab    lipgloss.Style
	inactiveTab  lipgloss.Style

	sparkSuccess lipgloss.TerminalColor
	sparkWarning lipgloss.TerminalColor
	sparkDanger  lipgloss.TerminalColor

	tableHeaderStyle   lipgloss.Style
	tableCellStyle     lipgloss.Style
	tableSelectedStyle lipgloss.Style
	tableBorderStyle   lipgloss.Style
	tableZebraStyle    lipgloss.Style

	siteGroupStyle lipgloss.Style
	maintStyle     lipgloss.Style
}

func newStyles(t Theme) *styles {
	return &styles{
		subtleStyle:  lipgloss.NewStyle().Foreground(t.Subtle).Faint(true),
		specialStyle: lipgloss.NewStyle().Foreground(t.Success),
		warnStyle:    lipgloss.NewStyle().Foreground(t.Warning).Bold(true),
		staleStyle:   lipgloss.NewStyle().Foreground(t.Stale).Faint(true),
		dangerStyle:  lipgloss.NewStyle().Foreground(t.Danger).Bold(true),
		titleStyle:   lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		activeTab:    lipgloss.NewStyle().Background(t.Surface).Foreground(t.Accent).Bold(true).Padding(0, 1),
		inactiveTab:  lipgloss.NewStyle().Padding(0, 1).Foreground(t.Muted).Faint(true),

		sparkSuccess: t.Success,
		sparkWarning: t.Warning,
		sparkDanger:  t.Danger,

		tableHeaderStyle:   lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Padding(0, 1),
		tableCellStyle:     lipgloss.NewStyle().Padding(0, 1),
		tableSelectedStyle: lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(t.SelectedFg).Background(t.SelectedBg),
		tableBorderStyle:   lipgloss.NewStyle().Foreground(t.Border).Faint(true),
		tableZebraStyle:    lipgloss.NewStyle().Padding(0, 1).Background(t.ZebraBg),

		siteGroupStyle: lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(t.Accent),
		maintStyle:     lipgloss.NewStyle().Foreground(t.Purple),
	}
}

var pulseFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	chromePadV   = 2 // outer Padding(1,2): 1 top + 1 bottom
	chromePadH   = 4 // outer Padding(1,2): 2 left + 2 right
	chromeHeader = 1 // tab bar line
	chromeGaps   = 2 // "\n" separators: before content + before footer
	chromeFooter = 2 // footer: "\n" prefix + text line
	chromeTable  = 3 // renderTable "\n" prefix + top border + header + bottom border (lipgloss collapses two into three rendered lines)
	chromeBase   = chromePadV + chromeHeader + chromeGaps + chromeFooter + chromeTable

	detailSparkWidth = 40
)

const (
	tabMonitors = 0
	tabMaint    = 1
	tabSettings = 2
)

const (
	sectionAlerts = 0
	sectionNodes  = 1
	sectionUsers  = 2
)

type sessionState int

const (
	stateDashboard sessionState = iota
	stateLogs
	stateUsers
	stateDetail
	stateAlertDetail
	stateFormSite
	stateFormAlert
	stateFormUser
	stateConfirmDelete
	stateFormMaint
	stateHistory
	stateSLA
)

type Model struct {
	state           sessionState
	currentTab      int
	settingsSection int
	cursor          int
	selectedID      int
	tableOffset     int
	maxTableRows    int
	termWidth       int
	termHeight      int
	contentWidth    int
	editID          int
	editToken       string

	huhForm       *huh.Form
	siteFormData  *siteFormData
	lastSiteType  string
	alertFormData *alertFormData
	userFormData  *userFormData
	maintFormData *maintFormData

	logViewport        viewport.Model
	logFilterImportant bool
	logTotal           int
	logShown           int

	historyViewport viewport.Model
	historyChanges  []models.StateChange
	historySiteName string
	historySiteID   int

	slaViewport       viewport.Model
	slaReport         monitor.SLAReport
	slaDailyBreakdown []monitor.DayReport
	slaSiteName       string
	slaSiteID         int
	slaPeriodIdx      int

	isAdmin bool
	zones   *zone.Manager

	deleteID   int
	deleteName string
	deleteTab  int

	collapsed  map[int]bool
	store      store.Store
	engine     *monitor.Engine
	theme      Theme
	themeIndex int
	st         *styles

	// harmonica animation state
	pulseSpring harmonica.Spring
	pulsePos    float64
	pulseVel    float64
	tickCount   int

	sites              []models.Site
	alerts             []models.AlertConfig
	users              []models.User
	nodes              []models.ProbeNode
	maintenanceWindows []models.MaintenanceWindow
	lastTabLoad        time.Time // last dispatch of loadTabDataCmd (throttle)
	tabSeq             int       // seq of the newest issued tab-data load

	// detail-panel state-change history, loaded on enter so View does no DB IO
	detailChanges       []models.StateChange
	detailChangesSiteID int

	filterMode bool
	filterText string

	sparkTooltipIdx int // clicked sparkline data index, -1 = none

	// demoMode renders a stable status dot instead of the animated pulse so
	// screenshots/recordings don't capture the spinner mid-frame. Set via UPTOP_DEMO=1.
	demoMode bool
	version  string
}

func InitialModel(isAdmin bool, s store.Store, eng *monitor.Engine, version string) Model {
	vpLogs := viewport.New(100, 20)
	vpLogs.SetContent("Waiting for logs...")
	z := zone.New()
	spring := harmonica.NewSpring(harmonica.FPS(10), 6.0, 0.4)
	collapsed := loadCollapsed(s)

	themeName, _ := s.GetPreference(context.Background(), "theme")
	theme := themeByName(themeName)
	themeIdx := 0
	for i, t := range themes {
		if t.Name == theme.Name {
			themeIdx = i
			break
		}
	}

	return Model{
		state:           stateDashboard,
		logViewport:     vpLogs,
		maxTableRows:    5,
		isAdmin:         isAdmin,
		store:           s,
		engine:          eng,
		zones:           z,
		pulseSpring:     spring,
		collapsed:       collapsed,
		theme:           theme,
		themeIndex:      themeIdx,
		st:              newStyles(theme),
		demoMode:        os.Getenv("UPTOP_DEMO") == "1",
		version:         version,
		sparkTooltipIdx: -1,
	}
}

// tickCmd schedules the next one-second heartbeat.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	// Load tab data immediately so the dashboard isn't empty for the first second.
	return tea.Batch(tea.ClearScreen, tickCmd(), m.loadTabDataCmd())
}
