package menu

import "github.com/charmbracelet/lipgloss"

var (
	// Base colors
	accent    = lipgloss.Color("63")  // purple-blue
	subtle    = lipgloss.Color("241") // gray
	highlight = lipgloss.Color("212") // pink
	fg        = lipgloss.Color("252") // light gray
	bg        = lipgloss.Color("234") // dark bg
	dim       = lipgloss.Color("240")
	green     = lipgloss.Color("42")
	red       = lipgloss.Color("196")

	// Layout
	containerStyle = lipgloss.NewStyle().
			Padding(0, 2)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(dim).
			MarginBottom(1)

	sectionStyle = lipgloss.NewStyle().
			Foreground(subtle).
			MarginTop(1).
			MarginBottom(0)

	// Category list items
	categorySelectedStyle = lipgloss.NewStyle().
				Foreground(fg).
				PaddingLeft(1)

	categoryUnselectedStyle = lipgloss.NewStyle().
				Foreground(dim).
				PaddingLeft(1)

	categoryTitleStyle = lipgloss.NewStyle().
				Bold(true)

	categoryDescStyle = lipgloss.NewStyle().
				Foreground(subtle).
				PaddingLeft(4)

	checkStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	crossStyle = lipgloss.NewStyle().
			Foreground(red).
			Bold(true)

	arrowStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true)

	// Detail view
	detailTitleStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true).
				MarginBottom(1)

	packageSelectedStyle = lipgloss.NewStyle().
				Foreground(fg).
				PaddingLeft(1)

	packageUnselectedStyle = lipgloss.NewStyle().
				Foreground(dim).
				PaddingLeft(1)

	packageDescStyle = lipgloss.NewStyle().
				Foreground(subtle).
				PaddingLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(dim).
			MarginTop(1)

	dividerStyle = lipgloss.NewStyle().
			Foreground(subtle)
)

func checkbox(selected bool) string {
	if selected {
		return checkStyle.Render("☑")
	}
	return crossStyle.Render("☐")
}

func pointer() string {
	return cursorStyle.Render("→")
}

func expandIcon() string {
	return arrowStyle.Render("▶")
}

func backIcon() string {
	return arrowStyle.Render("←")
}
