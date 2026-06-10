package tui

import (
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

var (
	subtleStyle  lipgloss.Style
	specialStyle lipgloss.Style
	warnStyle    lipgloss.Style
	staleStyle   lipgloss.Style
	dangerStyle  lipgloss.Style
	titleStyle   lipgloss.Style
	activeTab    lipgloss.Style
	inactiveTab  lipgloss.Style

	sparkSuccess string
	sparkWarning string
	sparkDanger  string
)

func applyTheme(t Theme) {
	subtleStyle = lipgloss.NewStyle().Foreground(t.Subtle)
	specialStyle = lipgloss.NewStyle().Foreground(t.Success)
	warnStyle = lipgloss.NewStyle().Foreground(t.Warning)
	staleStyle = lipgloss.NewStyle().Foreground(t.Stale)
	dangerStyle = lipgloss.NewStyle().Foreground(t.Danger)

	sparkSuccess = string(t.Success)
	sparkWarning = string(t.Warning)
	sparkDanger = string(t.Danger)

	titleStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	activeTab = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Accent).Bold(true).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Padding(0, 1).Foreground(t.Muted)

	tableHeaderStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Padding(0, 1)
	tableCellStyle = lipgloss.NewStyle().Padding(0, 1)
	tableSelectedStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(t.SelectedFg).Background(t.SelectedBg)
	tableBorderStyle = lipgloss.NewStyle().Foreground(t.Border)
	tableZebraStyle = lipgloss.NewStyle().Padding(0, 1).Background(t.ZebraBg)

	siteGroupStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(t.Accent)
	maintStyle = lipgloss.NewStyle().Foreground(t.Purple)
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
	state        sessionState
	currentTab   int
	cursor       int
	tableOffset  int
	maxTableRows int
	termWidth    int
	termHeight   int
	editID       int
	editToken    string

	huhForm       *huh.Form
	siteFormData  *siteFormData
	alertFormData *alertFormData
	userFormData  *userFormData
	maintFormData *maintFormData

	logViewport        viewport.Model
	logFilterImportant bool

	historyViewport viewport.Model
	historyChanges  []models.StateChange
	historySiteName string

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

	themeName, _ := s.GetPreference("theme")
	theme := themeByName(themeName)
	themeIdx := 0
	for i, t := range themes {
		if t.Name == theme.Name {
			themeIdx = i
			break
		}
	}

	applyTheme(theme)

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
		demoMode:        os.Getenv("UPTOP_DEMO") == "1",
		version:         version,
		sparkTooltipIdx: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.ClearScreen, tea.Tick(time.Second, func(t time.Time) tea.Msg { return t }))
}
