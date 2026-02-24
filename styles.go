package main

import (
	"github.com/charmbracelet/lipgloss"
)

// colors used throughout the app
var (
	colorRed     = lipgloss.Color("196")
	colorOrange  = lipgloss.Color("208")
	colorYellow  = lipgloss.Color("220")
	colorGreen   = lipgloss.Color("70")
	colorCyan    = lipgloss.Color("81")
	colorBlue    = lipgloss.Color("63")
	colorDim     = lipgloss.Color("240")
	colorDimmer  = lipgloss.Color("236")
	colorWhite   = lipgloss.Color("255")
	colorFg      = lipgloss.Color("252")
	colorAccent  = lipgloss.Color("81")
	colorSelBg   = lipgloss.Color("237")
	colorSelFg   = lipgloss.Color("255")
	colorDirIcon = lipgloss.Color("81")
)

// Style definitions
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan).
			Padding(0, 1)

	headerPathStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite)

	headerStatStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	headerDivider = lipgloss.NewStyle().
			Foreground(colorDim).
			SetString(" │ ")

	rowNumStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(4).
			Align(lipgloss.Right)

	percentStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(6).
			Align(lipgloss.Right)

	separatorStyle = lipgloss.NewStyle().
			Foreground(colorDimmer).
			SetString(" │ ")

	dirIconStyle = lipgloss.NewStyle().
			Foreground(colorDirIcon)

	fileIconStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	nameStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	sizeStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Align(lipgloss.Right)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(colorSelBg).
			Foreground(colorSelFg)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(0, 1)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true).
			Padding(0, 1)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(colorCyan)

	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan).
			Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Width(14)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	lineCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)

	searchPromptStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)
)

// barColor returns a color based on the percentage.
func barColor(pct float64) lipgloss.Color {
	switch {
	case pct >= 40:
		return colorRed
	case pct >= 20:
		return colorOrange
	case pct >= 10:
		return colorYellow
	case pct >= 2:
		return colorGreen
	default:
		return colorDim
	}
}
