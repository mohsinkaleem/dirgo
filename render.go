package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderHeader renders the top summary bar.
func renderHeader(m Model) string {
	path := headerPathStyle.Render(shortenPath(m.path))
	total := headerStatStyle.Render(fmt.Sprintf("Total: %s", formatSize(m.totalSize)))
	files := headerStatStyle.Render(fmt.Sprintf("%d files", m.totalFiles))
	dirs := headerStatStyle.Render(fmt.Sprintf("%d dirs", m.totalDirs))
	div := headerDivider.String()

	line := path + div + total + div + files + div + dirs

	if m.topMode {
		line += div + lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("TOP 10")
	}
	if m.dirOnly {
		line += div + lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("DIRS")
	}
	if m.showHidden {
		line += div + lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("HIDDEN")
	}
	if m.fromCache {
		line += div + lipgloss.NewStyle().Foreground(colorCyan).Render("⚡cached")
	}

	return headerStyle.Width(m.width).Render(line)
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
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
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
		rawMeta = fmt.Sprintf("%s l", formatCount(entry.LineCount))
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
	pointerSt := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	if !selected {
		pointerSt = lipgloss.NewStyle().Foreground(colorDim)
	}

	// Row number
	numStr := padLeft(fmt.Sprintf("%d.", index+1), 4)

	// Bar
	bc := barColor(entry.Percentage)
	barStr := barString(entry.Percentage, barMaxWidth)

	// Percentage
	pctStr := fmt.Sprintf("%5.1f%%", entry.Percentage)

	// Icon: directory ▸, symlink →, file space
	iconChar := "  "
	if entry.IsDir && entry.IsSymlink {
		iconChar = "⇢ "
	} else if entry.IsSymlink {
		iconChar = "→ "
	} else if entry.IsDir {
		iconChar = "▸ "
	}

	// Name (truncated + padded to fixed width)
	name := truncateStr(entry.Name, nameMaxWidth)
	name = padRight(name, nameMaxWidth)

	szStr := padLeft(formatSize(entry.Size), 9)

	// Assemble — apply selection background to each segment individually
	// so inner ANSI resets don't clobber the row background.

	parts := fmt.Sprintf("%s%s %s %s %s %s%s %s %s",
		styledSeg(pointer, pointerSt, false), // pointer has its own highlight
		styledSeg(numStr, lipgloss.NewStyle().Foreground(colorDim), selected),
		styledSeg(barStr, lipgloss.NewStyle().Foreground(bc), selected),
		styledSeg(pctStr, lipgloss.NewStyle().Foreground(colorDim).Width(6).Align(lipgloss.Right), selected),
		styledSeg("│", lipgloss.NewStyle().Foreground(colorDimmer), selected),
		styledSeg(iconChar, lipgloss.NewStyle().Foreground(colorDirIcon), selected),
		styledSeg(name, lipgloss.NewStyle().Foreground(func() lipgloss.Color {
			if selected {
				return colorSelFg
			}
			return colorFg
		}()).Bold(selected), selected),
		styledSeg(szStr, lipgloss.NewStyle().Foreground(colorDim), selected),
		styledSeg(rawMeta, lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true), selected),
	)

	// Pad row to full width and apply selection background to fill.
	visualW := lipgloss.Width(parts)
	if visualW < w {
		padding := strings.Repeat(" ", w-visualW)
		if selected {
			padding = lipgloss.NewStyle().Background(colorSelBg).Render(padding)
		}
		parts += padding
	}

	return parts
}

// renderFooter renders the bottom keybinding bar.
func renderFooter(m Model) string {
	if m.searchMode {
		return searchPromptStyle.Render(" / ") + m.searchInput.View()
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
		{"h", "hidden"},
		{"d", "dirs"},
		{"L", "lines"},
		{"?", "help"},
		{"q", "quit"},
	}

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = footerKeyStyle.Render(k.key) + " " + footerDescStyle.Render(k.desc)
	}

	return footerStyle.Width(m.width).Render(strings.Join(parts, "  "))
}

// renderHelp renders the help overlay.
func renderHelp(m Model) string {
	bindings := []struct {
		key  string
		desc string
	}{
		{"↑ / k", "Move cursor up"},
		{"↓ / j", "Move cursor down"},
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
		{"Esc", "Cancel search / close help"},
		{"h", "Toggle hidden files"},
		{"d", "Toggle directory-only view"},
		{"L", "Count lines for all files"},
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
