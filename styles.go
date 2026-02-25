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

	headerBadgeStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true)

	headerCachedStyle = lipgloss.NewStyle().
				Foreground(colorCyan)

	// Row styles (pre-defined to avoid per-row allocation)
	rowDimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	rowPctStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(6).
			Align(lipgloss.Right)

	rowSepStyle = lipgloss.NewStyle().
			Foreground(colorDimmer)

	rowIconStyle = lipgloss.NewStyle().
			Foreground(colorDirIcon)

	rowNameStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	rowNameSelStyle = lipgloss.NewStyle().
			Foreground(colorSelFg).
			Bold(true)

	rowMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)

	rowPointerActiveStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	rowPointerInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	rowSelBgStyle = lipgloss.NewStyle().
			Background(colorSelBg)

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

	searchPromptStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)
)

// Pre-defined bar color styles to avoid per-row allocation in renderRow.
var barStyles = map[lipgloss.Color]lipgloss.Style{
	colorRed:    lipgloss.NewStyle().Foreground(colorRed),
	colorOrange: lipgloss.NewStyle().Foreground(colorOrange),
	colorYellow: lipgloss.NewStyle().Foreground(colorYellow),
	colorGreen:  lipgloss.NewStyle().Foreground(colorGreen),
	colorDim:    lipgloss.NewStyle().Foreground(colorDim),
}

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
