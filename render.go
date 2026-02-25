package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// buildStatsLine assembles the stats/badges portion of the header (right-hand side).
func buildStatsLine(m Model) string {
	total := headerStatStyle.Render(fmt.Sprintf("Total: %s", formatSize(m.totalSize)))

	// Show deep totals if they differ from immediate counts
	fileStr := fmt.Sprintf("%d", m.totalFiles)
	if m.deepTotalFiles > int64(m.totalFiles) {
		fileStr += "/" + formatCount(int(m.deepTotalFiles))
	}
	fileStr += " files"
	files := headerStatStyle.Render(fileStr)

	dirStr := fmt.Sprintf("%d", m.totalDirs)
	if m.deepTotalDirs > int64(m.totalDirs) {
		dirStr += "/" + formatCount(int(m.deepTotalDirs))
	}
	dirStr += " dirs"
	dirs := headerStatStyle.Render(dirStr)

	div := headerDivider.String()

	statsLine := div + total + div + files + div + dirs
	if m.topMode {
		statsLine += div + headerBadgeStyle.Render("TOP 10")
	}
	switch m.viewFilter {
	case FilterDirsOnly:
		statsLine += div + headerBadgeStyle.Render("DIRS")
	case FilterFilesOnly:
		statsLine += div + headerBadgeStyle.Render("FILES")
	}
	if m.showHidden {
		statsLine += div + headerBadgeStyle.Render("HIDDEN")
	}
	if m.fromCache {
		statsLine += div + headerCachedStyle.Render("⚡cached")
	}
	return statsLine
}

// headerLineCount returns how many lines the header will occupy (1 or 2).
func headerLineCount(m Model) int {
	if m.width == 0 {
		return 1
	}
	statsLine := buildStatsLine(m)
	statsWidth := lipgloss.Width(statsLine)
	// headerStyle has Padding(0,1) = 2 horizontal chars
	maxPathWidth := m.width - statsWidth - 2
	if maxPathWidth >= 12 {
		return 1
	}
	return 2
}

// renderHeader renders the top summary bar.
func renderHeader(m Model) string {
	statsLine := buildStatsLine(m)
	statsWidth := lipgloss.Width(statsLine)

	// headerStyle has Padding(0,1) = 2 horizontal chars; leave 2 extra guard chars
	maxPathWidth := m.width - statsWidth - 2
	if maxPathWidth >= 12 {
		// Single-line: path ++ stats
		path := headerPathStyle.Render(truncatePath(shortenPath(m.path), maxPathWidth))
		return headerStyle.Width(m.width).Render(path + statsLine)
	}

	// Two-line: path on its own line, stats below
	pathWidth := m.width - 2 // account for padding
	if pathWidth < 1 {
		pathWidth = 1
	}
	path := headerPathStyle.Render(truncatePath(shortenPath(m.path), pathWidth))
	return headerStyle.Width(m.width).Render(path + "\n" + statsLine)
}

// styledSeg renders text with a style, optionally with a selection background applied too.
func styledSeg(text string, base lipgloss.Style, selected bool) string {
	if selected {
		return base.Background(colorSelBg).Render(text)
	}
	return base.Render(text)
}

// formatCount returns a compact number string: 999 → "999", 1234 → "1.2k", 1200000 → "1.2M".
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + "M"
	case n >= 1_000:
		return strconv.FormatFloat(float64(n)/1_000, 'f', 1, 64) + "k"
	default:
		return strconv.Itoa(n)
	}
}

// renderRow renders a single file entry row.
func renderRow(m Model, index int, entry FileEntry, selected bool) string {
	w := m.width
	if w < 40 {
		w = 40
	}

	// Fixed-width meta column (always 6 chars wide), only for text files: "1.2k l"
	const metaWidth = 6
	rawMeta := ""
	if !entry.IsDir && entry.LineCount > 0 {
		rawMeta = formatCount(entry.LineCount) + " l"
	}
	rawMeta = padLeft(truncateStr(rawMeta, metaWidth), metaWidth)

	// Fixed cols outside bar+name:
	// pointer(2) + num(4) + sp(1) + bar(var) + sp(1) + pct(6) + sp(1) + sep(1) + sp(1) + icon(2) + name(var) + sp(1) + sz(9) + sp(1) + meta(6)
	// = 2+4+1+1+6+1+1+1+2+1+9+1+6 = 36 fixed, plus bar and name.
	const fixedNonBar = 36
	barMaxWidth := maxInt(6, minInt(28, (w-fixedNonBar-16)/3))
	nameMaxWidth := maxInt(8, w-fixedNonBar-barMaxWidth)

	// Pointer
	pointer := "  "
	if selected {
		pointer = "▶ "
	}
	var pointerSt lipgloss.Style
	if selected {
		pointerSt = rowPointerActiveStyle
	} else {
		pointerSt = rowPointerInactiveStyle
	}

	// Row number
	numStr := padLeft(strconv.Itoa(index+1)+".", 4)

	// Bar
	bc := barColor(entry.Percentage)
	barStr := barString(entry.Percentage, barMaxWidth)

	// Percentage — use strconv to avoid fmt.Sprintf allocation
	pctStr := padLeft(strconv.FormatFloat(entry.Percentage, 'f', 1, 64)+"%%", 6)

	// Icon: directory ▸, symlink →, file space
	iconChar := "  "
	if entry.IsDir && entry.IsSymlink {
		iconChar = "⇢ "
	} else if entry.IsSymlink {
		iconChar = "→ "
	} else if entry.IsDir {
		iconChar = "▸ "
	}

	// Name (truncated + padded to fixed width, visual-width aware for emoji/wide chars)
	name := truncateStrVisual(entry.Name, nameMaxWidth)
	name = padRightVisual(name, nameMaxWidth)

	szStr := padLeft(formatSize(entry.Size), 9)

	// Select name style based on selection state
	var nameSt lipgloss.Style
	if selected {
		nameSt = rowNameSelStyle
	} else {
		nameSt = rowNameStyle
	}

	// Use pre-defined bar color style to avoid per-row allocation
	barSt := barStyles[bc]

	// Assemble — apply selection background to each segment individually
	// so inner ANSI resets don't clobber the row background.
	// Use string concatenation instead of fmt.Sprintf to reduce allocations.

	parts := styledSeg(pointer, pointerSt, false) +
		styledSeg(numStr, rowDimStyle, selected) + " " +
		styledSeg(barStr, barSt, selected) + " " +
		styledSeg(pctStr, rowPctStyle, selected) + " " +
		styledSeg("│", rowSepStyle, selected) + " " +
		styledSeg(iconChar, rowIconStyle, selected) +
		styledSeg(name, nameSt, selected) + " " +
		styledSeg(szStr, rowDimStyle, selected) + " " +
		styledSeg(rawMeta, rowMetaStyle, selected)

	// Pad row to full width and apply selection background to fill.
	visualW := lipgloss.Width(parts)
	if visualW < w {
		padding := strings.Repeat(" ", w-visualW)
		if selected {
			padding = rowSelBgStyle.Render(padding)
		}
		parts += padding
	}

	return parts
}

// renderFooter renders the bottom keybinding bar.
func renderFooter(m Model) string {
	if m.searchMode {
		return searchPromptStyle.Render(" / ") + m.searchInput.View() +
			footerDescStyle.Render("  ↑↓ nav  ⏎ apply  esc cancel")
	}

	if m.gotoMode {
		return searchPromptStyle.Render(" cd ") + m.gotoInput.View()
	}

	keys := []struct {
		key  string
		desc string
	}{
		{"↑↓", "nav"},
		{"←", "back"},
		{"→⏎", "open"},
		{"␣", "qlook"},
		{"r", "refresh"},
		{"t", "top10"},
		{"o", "finder"},
		{"/", "search"},
		{"c", "cd"},
		{"h", "hidden"},
		{"f", "filter"},
		{"d", "trash"},
		{"s", "lines"},
		{"?", "help"},
		{"q", "quit"},
	}

	// Use strings.Builder instead of slice+Join to reduce allocations
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(footerKeyStyle.Render(k.key))
		b.WriteString(" ")
		b.WriteString(footerDescStyle.Render(k.desc))
	}

	return footerStyle.Width(m.width).Render(b.String())
}

// renderHelp renders the help overlay.
func renderHelp(m Model) string {
	bindings := []struct {
		key  string
		desc string
	}{
		{"↑ / k", "Move cursor up"},
		{"↓ / j", "Move cursor down"},
		{"Scroll", "Mouse wheel up/down"},
		{"PgUp / ^U", "Page up"},
		{"PgDn / ^D", "Page down"},
		{"← / BS", "Go to parent (remembers position)"},
		{"→ / l / Enter", "Open dir / open file with default app"},
		{"Space", "Quick Look preview (macOS)"},
		{"g", "Jump to top of list"},
		{"G", "Jump to bottom of list"},
		{"r", "Refresh (smart — skips if unchanged)"},
		{"t", "Toggle top 10 view"},
		{"o", "Open in Finder (macOS)"},
		{"/", "Search / filter files"},
		{"↑↓ in /", "Navigate filtered results"},
		{"Esc", "Cancel search / close help"},
		{"c", "Go to directory (cd)"},
		{"h", "Toggle hidden files (on by default)"},
		{"f", "Cycle filter: all → dirs → files"},
		{"d", "Move selected entry to Trash"},
		{"s", "Count lines for all files"},
		{"?", "Show this help"},
		{"q / Ctrl+C", "Quit"},
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(helpTitleStyle.Render("  Keybindings"))
	b.WriteString("\n\n")

	for _, bind := range bindings {
		b.WriteString("  ")
		b.WriteString(helpKeyStyle.Render(bind.key))
		b.WriteString(helpDescStyle.Render(bind.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("  Press Esc or ? to close"))
	b.WriteString("\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(0, 2).
		Width(minInt(50, m.width-4))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(b.String()))
}
