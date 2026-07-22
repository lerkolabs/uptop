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
	themeEverforest,
	themeKanagawa,
	themeTokyoNight,
	themeCatppuccinMocha,
	themeRosePine,
	themeDracula,
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
	Muted:      cc("#7982a9", "7"),
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
	SelectedBg: cc("#363c53", "4"),
}

var themeDracula = Theme{
	Name:       "Dracula",
	Bg:         cc("#282a36", ""),
	Surface:    cc("#343746", ""),
	Panel:      cc("#44475a", ""),
	Border:     cc("#6272a4", "8"),
	Fg:         cc("#f8f8f2", "15"),
	Muted:      cc("#a9b0cb", "7"),
	Subtle:     cc("#6272a4", "7"),
	Success:    cc("#50fa7b", "10"),
	Warning:    cc("#f1fa8c", "11"),
	Stale:      cc("#ffb86c", "3"),
	Danger:     cc("#ff5555", "9"),
	Info:       cc("#8be9fd", "12"),
	Accent:     cc("#bd93f9", "14"),
	Purple:     cc("#ff79c6", "13"),
	ZebraBg:    cc("#2c2e3a", ""),
	SelectedFg: cc("#f8f8f2", "15"),
	SelectedBg: cc("#52556b", "4"),
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
	Success:    cc("#7dc47a", "10"),
	Warning:    cc("#f0c644", "11"),
	Stale:      cc("#fab387", "3"),
	Danger:     cc("#e6546e", "9"),
	Info:       cc("#89b4fa", "12"),
	Accent:     cc("#94e2d5", "14"),
	Purple:     cc("#cba6f7", "13"),
	ZebraBg:    cc("#212130", ""),
	SelectedFg: cc("#cdd6f4", "15"),
	SelectedBg: cc("#585b70", "4"),
}

var themeRosePine = Theme{
	Name:       "Rosé Pine",
	Bg:         cc("#191724", ""),
	Surface:    cc("#1f1d2e", ""),
	Panel:      cc("#26233a", ""),
	Border:     cc("#524f67", "8"),
	Fg:         cc("#e0def4", "15"),
	Muted:      cc("#908caa", "7"),
	Subtle:     cc("#6e6a86", "7"),
	Success:    cc("#9ccfd8", "10"),
	Warning:    cc("#f6c177", "11"),
	Stale:      cc("#ebbcba", "3"),
	Danger:     cc("#eb6f92", "9"),
	Info:       cc("#3e8fb0", "12"),
	Accent:     cc("#c4a7e7", "14"),
	Purple:     cc("#ebbcba", "13"),
	ZebraBg:    cc("#1c1a28", ""),
	SelectedFg: cc("#e0def4", "15"),
	SelectedBg: cc("#403d52", "4"),
}

var themeKanagawa = Theme{
	Name:       "Kanagawa",
	Bg:         cc("#1F1F28", ""),
	Surface:    cc("#2A2A37", ""),
	Panel:      cc("#363646", ""),
	Border:     cc("#54546D", "8"),
	Fg:         cc("#DCD7BA", "15"),
	Muted:      cc("#C8C093", "7"),
	Subtle:     cc("#727169", "7"),
	Success:    cc("#98BB6C", "10"),
	Warning:    cc("#E6C384", "11"),
	Stale:      cc("#FFA066", "3"),
	Danger:     cc("#E46876", "9"),
	Info:       cc("#7E9CD8", "12"),
	Accent:     cc("#7FB4CA", "14"),
	Purple:     cc("#D27E99", "13"),
	ZebraBg:    cc("#22222c", ""),
	SelectedFg: cc("#DCD7BA", "15"),
	SelectedBg: cc("#2D4F67", "4"),
}

var themeEverforest = Theme{
	Name:       "Everforest",
	Bg:         cc("#2F383E", ""),
	Surface:    cc("#343F44", ""),
	Panel:      cc("#3D484D", ""),
	Border:     cc("#56635F", "8"),
	Fg:         cc("#D3C6AA", "15"),
	Muted:      cc("#9DA9A0", "7"),
	Subtle:     cc("#7A8478", "7"),
	Success:    cc("#A7C080", "10"),
	Warning:    cc("#DBBC7F", "11"),
	Stale:      cc("#E69875", "3"),
	Danger:     cc("#E67E80", "9"),
	Info:       cc("#7FBBB3", "12"),
	Accent:     cc("#83C092", "14"),
	Purple:     cc("#D699B6", "13"),
	ZebraBg:    cc("#30393F", ""),
	SelectedFg: cc("#D3C6AA", "15"),
	SelectedBg: cc("#475258", "4"),
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
