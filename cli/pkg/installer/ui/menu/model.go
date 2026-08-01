package menu

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Result holds the user's selections after the menu completes.
type Result struct {
	Categories []Category
	Groups     []string
}

// State represents the current view.
type state int

const (
	stateCategories state = iota
	stateDetail
	stateDone
)

type tickMsg struct{}

// Model is the Bubble Tea model for the installer menu.
type Model struct {
	width        int
	height       int
	categories   []Category
	cursor       int
	detailCursor int
	state        state
	detailIdx    int
	result       *Result
}

// New creates a new menu model with the given categories.
func New(categories []Category) Model {
	return Model{
		categories:   categories,
		cursor:       0,
		detailCursor: 0,
		state:        stateCategories,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case stateCategories:
			return m.updateCategories(msg)
		case stateDetail:
			return m.updateDetail(msg)
		}
	}
	return m, nil
}

func (m Model) updateCategories(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		// Cancel = empty result
		m.result = &Result{}
		m.state = stateDone
		return m, tea.Quit

	case "q":
		m.result = &Result{}
		m.state = stateDone
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.categories)-1 {
			m.cursor++
		}

	case " ":
		// Toggle entire category on/off
		cat := &m.categories[m.cursor]
		allSelected := true
		for _, pkg := range cat.Packages {
			if !pkg.Selected {
				allSelected = false
				break
			}
		}
		newVal := !allSelected
		for i := range cat.Packages {
			cat.Packages[i].Selected = newVal
		}

	case "enter", "right", "l":
		// Enter detail view
		if len(m.categories[m.cursor].Packages) > 0 {
			m.state = stateDetail
			m.detailIdx = m.cursor
			m.detailCursor = 0
		}

	case "y", "Y":
		// Confirm and finish
		m.result = &Result{
			Categories: m.categories,
			Groups:     SelectedGroups(m.categories),
		}
		m.state = stateDone
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cat := &m.categories[m.detailIdx]

	switch msg.String() {
	case "ctrl+c", "esc", "left", "h", "backspace":
		m.state = stateCategories
		m.detailCursor = 0
		return m, nil

	case "up", "k":
		if m.detailCursor > 0 {
			m.detailCursor--
		}

	case "down", "j":
		if m.detailCursor < len(cat.Packages)-1 {
			m.detailCursor++
		}

	case " ":
		// Toggle individual package
		pkg := &cat.Packages[m.detailCursor]
		pkg.Selected = !pkg.Selected
	}

	return m, nil
}

func (m Model) View() string {
	switch m.state {
	case stateCategories:
		return m.viewCategories()
	case stateDetail:
		return m.viewDetail()
	default:
		return ""
	}
}

func (m Model) viewCategories() string {
	var b strings.Builder

	// Header
	header := borderStyle.Render(
		banner() + "\n" +
			subtitleStyle.Render("Arch Linux • Hyprland • Tokyo Night"),
	)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Core system note
	b.WriteString(sectionStyle.Render("Core System (always installed)"))
	b.WriteString("\n")
	b.WriteString(dimmed("  zsh  stow  git  base-devel  fonts  icons  moonarch-themes"))
	b.WriteString("\n\n")
	b.WriteString(divider("─", 60))
	b.WriteString("\n\n")

	// Category list
	for i, cat := range m.categories {
		cursor := "  "
		if m.cursor == i {
			cursor = pointer() + " "
		}

		allOff := true
		allOn := true
		for _, pkg := range cat.Packages {
			if pkg.Selected {
				allOff = false
			} else {
				allOn = false
			}
		}

		var check string
		if allOn {
			check = checkStyle.Render("☑")
		} else if allOff {
			check = crossStyle.Render("☐")
		} else {
			check = checkStyle.Render("◐")
		}

		title := cat.Title
		if m.cursor == i {
			title = cursorStyle.Render(title)
		}

		fmt.Fprintf(&b, "%s%s %s", cursor, check, title)

		if len(cat.Packages) > 0 {
			b.WriteString("  " + expandIcon())
		}

		b.WriteString("\n")

		// Show description for cursor item
		if m.cursor == i {
			b.WriteString(categoryDescStyle.Render(cat.Description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(divider("─", 60))
	b.WriteString("\n\n")

	// Footer actions
	selectedCount := len(SelectedGroups(m.categories))
	b.WriteString(fmt.Sprintf("  Groups selected: %d/%d\n", selectedCount, len(m.categories)))

	b.WriteString(helpStyle.Render("  ↑↓ navigate   space toggle   enter details   y confirm   esc quit"))
	b.WriteString("\n")

	return containerStyle.Render(b.String())
}

func (m Model) viewDetail() string {
	cat := m.categories[m.detailIdx]

	var b strings.Builder

	// Header
	header := borderStyle.Render(
		detailTitleStyle.Render(cat.Title) + "\n" +
			dimmed(cat.Description),
	)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Package list
	for i, pkg := range cat.Packages {
		cursor := "  "
		if m.detailCursor == i {
			cursor = pointer() + " "
		}

		check := checkbox(pkg.Selected)

		name := pkg.Name
		if m.detailCursor == i {
			name = cursorStyle.Render(name)
		}

		fmt.Fprintf(&b, "%s%s %s\n", cursor, check, name)

		if m.detailCursor == i {
			b.WriteString(packageDescStyle.Render(pkg.Description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(divider("─", 60))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("  ↑↓ navigate   space toggle   esc/h back"))
	b.WriteString("\n")

	return containerStyle.Render(b.String())
}

// Result returns the menu selections, or nil if the user hasn't confirmed.
func (m Model) Result() *Result { return m.result }

func dimmed(s string) string {
	return lipgloss.NewStyle().Foreground(dim).Render(s)
}

func divider(char string, width int) string {
	return lipgloss.NewStyle().
		Foreground(subtle).
		Render(strings.Repeat(char, width))
}

func banner() string {
	return accentStyle().Render(
		"███╗   ███╗ ██████╗  ██████╗ ███╗   ██╗ █████╗ ██████╗  ██████╗██╗  ██╗\n" +
			"████╗ ████║██╔═══██╗██╔═══██╗████╗  ██║██╔══██╗██╔══██╗██╔════╝██║  ██║\n" +
			"██╔████╔██║██║   ██║██║   ██║██╔██╗ ██║███████║██████╔╝██║     ███████║\n" +
			"██║╚██╔╝██║██║   ██║██║   ██║██║╚██╗██║██╔══██║██╔══██╗██║     ██╔══██║\n" +
			"██║ ╚═╝ ██║╚██████╔╝╚██████╔╝██║ ╚████║██║  ██║██║  ██║╚██████╗██║  ██║\n" +
			"╚═╝     ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝",
	)
}

func accentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(accent)
}
