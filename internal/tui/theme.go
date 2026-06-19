package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func cc(hex, ansi string) lipgloss.CompleteColor {
	return lipgloss.CompleteColor{
		TrueColor: hex,
		ANSI256:   hex,
		ANSI:      ansi,
	}
}

type Theme struct {
	Name string

	// Base layers
	Bg      lipgloss.TerminalColor
	Surface lipgloss.TerminalColor
	Panel   lipgloss.TerminalColor
	Border  lipgloss.TerminalColor

	// Text
	Fg     lipgloss.TerminalColor
	Muted  lipgloss.TerminalColor
	Subtle lipgloss.TerminalColor

	// Semantic
	Success lipgloss.TerminalColor
	Warning lipgloss.TerminalColor
	Stale   lipgloss.TerminalColor
	Danger  lipgloss.TerminalColor
	Info    lipgloss.TerminalColor
	Accent  lipgloss.TerminalColor
	Purple  lipgloss.TerminalColor

	// Table
	ZebraBg lipgloss.TerminalColor

	// Selection
	SelectedFg lipgloss.TerminalColor
	SelectedBg lipgloss.TerminalColor
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
	Bg:         cc("#1C1B1A", ""),
	Surface:    cc("#282726", ""),
	Panel:      cc("#343331", ""),
	Border:     cc("#575653", "8"),
	Fg:         cc("#CECDC3", "15"),
	Muted:      cc("#878580", "7"),
	Subtle:     cc("#6F6E69", "7"),
	Success:    cc("#879A39", "10"),
	Warning:    cc("#D0A215", "11"),
	Stale:      cc("#DA702C", "3"),
	Danger:     cc("#D14D41", "9"),
	Info:       cc("#4385BE", "12"),
	Accent:     cc("#3AA99F", "14"),
	Purple:     cc("#8B7EC8", "13"),
	ZebraBg:    cc("#222120", ""),
	SelectedFg: cc("#FFFCF0", "15"),
	SelectedBg: cc("#403E3C", "4"),
}

var themeTokyoNight = Theme{
	Name:       "Tokyo Night",
	Bg:         cc("#1a1b26", ""),
	Surface:    cc("#24283b", ""),
	Panel:      cc("#292e42", ""),
	Border:     cc("#3b4261", "8"),
	Fg:         cc("#c0caf5", "15"),
	Muted:      cc("#a9b1d6", "7"),
	Subtle:     cc("#565f89", "7"),
	Success:    cc("#9ece6a", "10"),
	Warning:    cc("#e0af68", "11"),
	Stale:      cc("#ff9e64", "3"),
	Danger:     cc("#f7768e", "9"),
	Info:       cc("#7aa2f7", "12"),
	Accent:     cc("#7dcfff", "14"),
	Purple:     cc("#bb9af7", "13"),
	ZebraBg:    cc("#1c1d28", ""),
	SelectedFg: cc("#c0caf5", "15"),
	SelectedBg: cc("#292e42", "4"),
}

var themeGruvbox = Theme{
	Name:       "Gruvbox",
	Bg:         cc("#282828", ""),
	Surface:    cc("#3c3836", ""),
	Panel:      cc("#504945", ""),
	Border:     cc("#665c54", "8"),
	Fg:         cc("#ebdbb2", "15"),
	Muted:      cc("#bdae93", "7"),
	Subtle:     cc("#7c6f64", "7"),
	Success:    cc("#b8bb26", "10"),
	Warning:    cc("#fabd2f", "11"),
	Stale:      cc("#fe8019", "3"),
	Danger:     cc("#fb4934", "9"),
	Info:       cc("#83a598", "12"),
	Accent:     cc("#8ec07c", "14"),
	Purple:     cc("#d3869b", "13"),
	ZebraBg:    cc("#2a2a2a", ""),
	SelectedFg: cc("#fbf1c7", "15"),
	SelectedBg: cc("#504945", "4"),
}

var themeCatppuccinMocha = Theme{
	Name:       "Catppuccin Mocha",
	Bg:         cc("#1e1e2e", ""),
	Surface:    cc("#313244", ""),
	Panel:      cc("#45475a", ""),
	Border:     cc("#585b70", "8"),
	Fg:         cc("#cdd6f4", "15"),
	Muted:      cc("#a6adc8", "7"),
	Subtle:     cc("#6c7086", "7"),
	Success:    cc("#a6e3a1", "10"),
	Warning:    cc("#f9e2af", "11"),
	Stale:      cc("#fab387", "3"),
	Danger:     cc("#f38ba8", "9"),
	Info:       cc("#89b4fa", "12"),
	Accent:     cc("#94e2d5", "14"),
	Purple:     cc("#cba6f7", "13"),
	ZebraBg:    cc("#232334", ""),
	SelectedFg: cc("#cdd6f4", "15"),
	SelectedBg: cc("#45475a", "4"),
}

var themeNord = Theme{
	Name:       "Nord",
	Bg:         cc("#2e3440", ""),
	Surface:    cc("#3b4252", ""),
	Panel:      cc("#434c5e", ""),
	Border:     cc("#4c566a", "8"),
	Fg:         cc("#d8dee9", "15"),
	Muted:      cc("#d8dee9", "7"),
	Subtle:     cc("#4c566a", "7"),
	Success:    cc("#a3be8c", "10"),
	Warning:    cc("#ebcb8b", "11"),
	Stale:      cc("#d08770", "3"),
	Danger:     cc("#bf616a", "9"),
	Info:       cc("#81a1c1", "12"),
	Accent:     cc("#88c0d0", "14"),
	Purple:     cc("#b48ead", "13"),
	ZebraBg:    cc("#323845", ""),
	SelectedFg: cc("#eceff4", "15"),
	SelectedBg: cc("#434c5e", "4"),
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
