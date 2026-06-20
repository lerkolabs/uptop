package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type userFormData struct {
	Username  string
	PublicKey string
	Role      string
}

func (m Model) fmtRole(role string) string {
	if role == "admin" {
		return m.st.specialStyle.Render(role)
	}
	return role
}

func fmtKey(key string) string {
	if len(key) > 40 {
		return key[:20] + "..." + key[len(key)-17:]
	}
	return key
}

func (m Model) viewUsersTab() string {
	if len(m.users) == 0 {
		return m.emptyState("No users configured.", "[n] Add a user")
	}

	var headers []string
	var widths []int
	if m.isWide() {
		headers = []string{"#", "USERNAME", "ROLE", "PUBLIC KEY"}
		widths = []int{4, 18, 10, 50}
	} else {
		headers = []string{"#", "USER", "ROLE", "KEY"}
		widths = []int{4, 14, 8, 30}
	}
	userW := widths[1]

	return m.renderTable(
		headers,
		len(m.users),
		func(start, end int) [][]string {
			var rows [][]string
			for i := start; i < end; i++ {
				u := m.users[i]
				rows = append(rows, []string{
					fmt.Sprintf("%d", i+1),
					m.zones.Mark(fmt.Sprintf("user-%d", i), limitStr(u.Username, userW-2)),
					m.fmtRole(u.Role),
					fmtKey(u.PublicKey),
				})
			}
			return rows
		},
		widths, nil,
	)
}

func (m *Model) initUserHuhForm() tea.Cmd {
	m.userFormData = &userFormData{
		Role: "user",
	}

	if m.editID > 0 {
		for _, u := range m.users {
			if u.ID == m.editID {
				m.userFormData.Username = u.Username
				m.userFormData.PublicKey = u.PublicKey
				m.userFormData.Role = u.Role
				break
			}
		}
	}

	m.huhForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Username").
				Placeholder("admin").
				Value(&m.userFormData.Username).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("username is required")
					}
					return nil
				}),
			huh.NewInput().Title("SSH Public Key").
				Placeholder("ssh-ed25519 AAAA...").
				Value(&m.userFormData.PublicKey).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("public key is required")
					}
					return nil
				}),
			huh.NewSelect[string]().Title("Role").
				Options(
					huh.NewOption("User", "user"),
					huh.NewOption("Admin", "admin"),
				).Value(&m.userFormData.Role),
		).Title("SSH Access"),
	).WithTheme(m.theme.HuhTheme())

	return m.huhForm.Init()
}

func (m *Model) submitUserForm() tea.Cmd {
	d := m.userFormData
	st := m.store
	id := m.editID
	username, key, role := d.Username, d.PublicKey, d.Role
	m.state = stateDashboard
	if id > 0 {
		return writeCmd("Update user", func() error {
			return st.UpdateUser(context.Background(), id, username, key, role)
		})
	}
	return writeCmd("Add user", func() error {
		return st.AddUser(context.Background(), username, key, role)
	})
}
