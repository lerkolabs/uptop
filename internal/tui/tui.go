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
)

const (
	sectionAlerts = 0
	sectionNodes  = 1
	sectionUsers  = 2
)

const (
	panelMonitors = 0
	panelLogs     = 1
	panelDetail   = 2
	panelMaint    = 3
)

type bottomPanel int

const (
	bottomNone bottomPanel = iota
	bottomLogs
	bottomMaint
)

const (
	detailDefault = 0
	detailSLA     = 1
	detailHistory = 2
)

const (
	sortStatus  = 0
	sortName    = 1
	sortLatency = 2
	sortMax     = 3
)

type sessionState int

const (
	stateDashboard        sessionState = 0
	stateLogs             sessionState = 1
	stateDetailFullscreen sessionState = 2
	stateAlertDetail      sessionState = 3
	stateFormSite         sessionState = 4
	stateFormAlert        sessionState = 5
	stateFormUser         sessionState = 6
	stateConfirmDelete    sessionState = 7
	stateFormMaint        sessionState = 8
	stateSettings         sessionState = 11
	stateMaintDetail      sessionState = 12
)

type Model struct {
	state           sessionState
	returnState     sessionState
	settingsSection int
	settingsCursor  int
	settingsOffset  int
	cursor          int
	selectedID      int
	sortColumn      int
	sortAsc         bool
	tableOffset     int
	maxTableRows    int
	termWidth       int
	termHeight      int
	contentWidth    int
	focusedPanel    int
	detailMode      int
	logScrollOffset int
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

	historyChanges  []models.StateChange
	historySiteName string
	historySiteID   int

	slaReport          monitor.SLAReport
	slaDailyBreakdown  []monitor.DayReport
	slaSiteName        string
	slaSiteID          int
	slaPeriodIdx       int
	detailScrollOffset int

	isAdmin bool
	zones   *zone.Manager

	deleteID   int
	deleteName string
	deleteKind string

	maintDetailID int

	ctx        context.Context
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

	maintSet            map[int]bool
	bottomPanel         bottomPanel
	detailOpen          bool
	maintCursor         int
	detailChanges       []models.StateChange
	detailChangesSiteID int
	detailDailyDays     []monitor.DayReport

	filterMode bool
	filterText string

	// demoMode renders a stable status dot instead of the animated pulse so
	// screenshots/recordings don't capture the spinner mid-frame. Set via UPTOP_DEMO=1.
	demoMode bool
	version  string
}

func InitialModel(ctx context.Context, isAdmin bool, s store.Store, eng *monitor.Engine, version string) Model {
	vpLogs := viewport.New(100, 20)
	vpLogs.SetContent("Waiting for logs...")
	z := zone.New()
	spring := harmonica.NewSpring(harmonica.FPS(10), 6.0, 0.4)
	collapsed := loadCollapsed(ctx, s)

	themeName, _ := s.GetPreference(ctx, "theme")
	theme := themeByName(themeName)
	themeIdx := 0
	for i, t := range themes {
		if t.Name == theme.Name {
			themeIdx = i
			break
		}
	}

	detailPref, _ := s.GetPreference(ctx, "detail_open")

	bp := bottomLogs
	if bpPref, _ := s.GetPreference(ctx, "bottom_panel"); bpPref != "" {
		switch bpPref {
		case "none":
			bp = bottomNone
		case "maint":
			bp = bottomMaint
		}
	}

	return Model{
		ctx:          ctx,
		state:        stateDashboard,
		logViewport:  vpLogs,
		maxTableRows: 5,
		isAdmin:      isAdmin,
		store:        s,
		engine:       eng,
		zones:        z,
		pulseSpring:  spring,
		collapsed:    collapsed,
		theme:        theme,
		themeIndex:   themeIdx,
		st:           newStyles(theme),
		bottomPanel:  bp,
		detailOpen:   detailPref == "true",
		demoMode:     os.Getenv("UPTOP_DEMO") == "1",
		version:      version,
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
