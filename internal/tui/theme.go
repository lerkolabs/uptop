package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Name string

	// Base layers
	Bg      lipgloss.Color
	Surface lipgloss.Color
	Panel   lipgloss.Color
	Border  lipgloss.Color

	// Text
	Fg     lipgloss.Color
	Muted  lipgloss.Color
	Subtle lipgloss.Color

	// Semantic
	Success lipgloss.Color
	Warning lipgloss.Color
	Stale   lipgloss.Color
	Danger  lipgloss.Color
	Info    lipgloss.Color
	Accent  lipgloss.Color
	Purple  lipgloss.Color

	// Table
	ZebraBg lipgloss.Color

	// Selection
	SelectedFg lipgloss.Color
	SelectedBg lipgloss.Color
}

var themes = []Theme{
	themeFlexokiDark,
	themeTokyoNight,
	themeCatppuccinMocha,
	themeNord,
	themeGruvbox,
}

var themeFlexokiDark = Theme{
	Name:       "Flexoki Dark",
	Bg:         "#1C1B1A",
	Surface:    "#282726",
	Panel:      "#343331",
	Border:     "#575653",
	Fg:         "#CECDC3",
	Muted:      "#878580",
	Subtle:     "#6F6E69",
	Success:    "#879A39",
	Warning:    "#D0A215",
	Stale:      "#DA702C",
	Danger:     "#D14D41",
	Info:       "#4385BE",
	Accent:     "#3AA99F",
	Purple:     "#8B7EC8",
	ZebraBg:    "#222120",
	SelectedFg: "#FFFCF0",
	SelectedBg: "#403E3C",
}

var themeTokyoNight = Theme{
	Name:       "Tokyo Night",
	Bg:         "#1a1b26",
	Surface:    "#24283b",
	Panel:      "#292e42",
	Border:     "#3b4261",
	Fg:         "#c0caf5",
	Muted:      "#a9b1d6",
	Subtle:     "#565f89",
	Success:    "#9ece6a",
	Warning:    "#e0af68",
	Stale:      "#ff9e64",
	Danger:     "#f7768e",
	Info:       "#7aa2f7",
	Accent:     "#7dcfff",
	Purple:     "#bb9af7",
	ZebraBg:    "#1c1d28",
	SelectedFg: "#c0caf5",
	SelectedBg: "#292e42",
}

var themeGruvbox = Theme{
	Name:       "Gruvbox",
	Bg:         "#282828",
	Surface:    "#3c3836",
	Panel:      "#504945",
	Border:     "#665c54",
	Fg:         "#ebdbb2",
	Muted:      "#bdae93",
	Subtle:     "#7c6f64",
	Success:    "#b8bb26",
	Warning:    "#fabd2f",
	Stale:      "#fe8019",
	Danger:     "#fb4934",
	Info:       "#83a598",
	Accent:     "#8ec07c",
	Purple:     "#d3869b",
	ZebraBg:    "#2a2a2a",
	SelectedFg: "#fbf1c7",
	SelectedBg: "#504945",
}

var themeCatppuccinMocha = Theme{
	Name:       "Catppuccin Mocha",
	Bg:         "#1e1e2e",
	Surface:    "#313244",
	Panel:      "#45475a",
	Border:     "#585b70",
	Fg:         "#cdd6f4",
	Muted:      "#a6adc8",
	Subtle:     "#6c7086",
	Success:    "#a6e3a1",
	Warning:    "#f9e2af",
	Stale:      "#fab387",
	Danger:     "#f38ba8",
	Info:       "#89b4fa",
	Accent:     "#94e2d5",
	Purple:     "#cba6f7",
	ZebraBg:    "#232334",
	SelectedFg: "#cdd6f4",
	SelectedBg: "#45475a",
}

var themeNord = Theme{
	Name:       "Nord",
	Bg:         "#2e3440",
	Surface:    "#3b4252",
	Panel:      "#434c5e",
	Border:     "#4c566a",
	Fg:         "#d8dee9",
	Muted:      "#d8dee9",
	Subtle:     "#4c566a",
	Success:    "#a3be8c",
	Warning:    "#ebcb8b",
	Stale:      "#d08770",
	Danger:     "#bf616a",
	Info:       "#81a1c1",
	Accent:     "#88c0d0",
	Purple:     "#b48ead",
	ZebraBg:    "#323845",
	SelectedFg: "#eceff4",
	SelectedBg: "#434c5e",
}

func (t Theme) HuhTheme() *huh.Theme {
	ht := huh.ThemeBase()

	ht.Focused.Base = ht.Focused.Base.BorderForeground(t.Border)
	ht.Focused.Card = ht.Focused.Base
	ht.Focused.Title = ht.Focused.Title.Foreground(t.Accent).Bold(true)
	ht.Focused.NoteTitle = ht.Focused.NoteTitle.Foreground(t.Accent).Bold(true).MarginBottom(1)
	ht.Focused.Description = ht.Focused.Description.Foreground(t.Muted)
	ht.Focused.ErrorIndicator = ht.Focused.ErrorIndicator.Foreground(t.Danger)
	ht.Focused.ErrorMessage = ht.Focused.ErrorMessage.Foreground(t.Danger)
	ht.Focused.SelectSelector = ht.Focused.SelectSelector.Foreground(t.Purple)
	ht.Focused.NextIndicator = ht.Focused.NextIndicator.Foreground(t.Purple)
	ht.Focused.PrevIndicator = ht.Focused.PrevIndicator.Foreground(t.Purple)
	ht.Focused.Option = ht.Focused.Option.Foreground(t.Fg)
	ht.Focused.MultiSelectSelector = ht.Focused.MultiSelectSelector.Foreground(t.Purple)
	ht.Focused.SelectedOption = ht.Focused.SelectedOption.Foreground(t.Success)
	ht.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(t.Success).SetString("✓ ")
	ht.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(t.Subtle).SetString("• ")
	ht.Focused.UnselectedOption = ht.Focused.UnselectedOption.Foreground(t.Fg)
	ht.Focused.FocusedButton = ht.Focused.FocusedButton.Foreground(t.Bg).Background(t.Accent)
	ht.Focused.Next = ht.Focused.FocusedButton
	ht.Focused.BlurredButton = ht.Focused.BlurredButton.Foreground(t.Fg).Background(t.Surface)
	ht.Focused.TextInput.Cursor = ht.Focused.TextInput.Cursor.Foreground(t.Accent)
	ht.Focused.TextInput.Placeholder = ht.Focused.TextInput.Placeholder.Foreground(t.Subtle)
	ht.Focused.TextInput.Prompt = ht.Focused.TextInput.Prompt.Foreground(t.Purple)

	ht.Blurred = ht.Focused
	ht.Blurred.Base = ht.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	ht.Blurred.Card = ht.Blurred.Base
	ht.Blurred.NextIndicator = lipgloss.NewStyle()
	ht.Blurred.PrevIndicator = lipgloss.NewStyle()

	ht.Group.Title = ht.Focused.Title
	ht.Group.Description = ht.Focused.Description

	return ht
}

func themeByName(name string) Theme {
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return themes[0]
}
