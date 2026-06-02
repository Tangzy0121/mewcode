package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mewcode/internal/permission"
)

var (
	userStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")) // blue
	assistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")) // green
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // grey
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // red
	promptStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // yellow

	cardColor = map[string]string{
		"running": "11", // yellow
		"ok":      "10", // green
		"error":   "9",  // red
	}
	cardGlyph = map[string]string{
		"running": "⏳",
		"ok":      "✓",
		"error":   "✗",
	}
)

func dim(s string) string     { return dimStyle.Render(s) }
func errLine(s string) string { return errStyle.Render(s) }

// renderCard draws a tool card colored by status.
func renderCard(c *toolCard) string {
	color := cardColor[c.status]
	if color == "" {
		color = "8"
	}
	glyph := cardGlyph[c.status]
	if glyph == "" {
		glyph = "•"
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(color)).
		Padding(0, 1)
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).
		Render(fmt.Sprintf("%s %s", glyph, c.name))
	body := head
	if c.summary != "" {
		body += "\n" + dim(truncate(c.summary, 80))
	}
	return style.Render(body)
}

// renderPermission draws the approval prompt shown in place of the input box.
func renderPermission(req *permission.PendingRequest) string {
	var b strings.Builder
	b.WriteString(promptStyle.Render(fmt.Sprintf("✋ allow %s?", req.ToolName)))
	b.WriteString("  " + dim(truncate(oneLine(string(req.Input)), 60)))
	b.WriteString("\n")
	b.WriteString("  [a] allow once   [s] always allow   [d] deny")
	b.WriteString("\n")
	return b.String()
}

func divider(width int) string {
	if width <= 0 {
		width = 40
	}
	return dim(strings.Repeat("─", width)) + "\n"
}

// tailLines keeps only the last n lines so the body fits the terminal.
// asciiCat is the classic pure-ASCII kitten shown on the welcome screen. Every
// glyph is a single-cell ASCII char, so it aligns identically on any terminal.
func asciiCat() string {
	art := " /\\_/\\\n( o.o )\n > ^ <"
	return lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Render(art)
}

// catBanner is pinned to the top of the conversation (see bodyContent).
func catBanner() string {
	title := assistantStyle.Render("MewCode") + dim("  ~ a tiny terminal coding cat")
	hint := dim("PgUp/PgDn or mouse wheel to scroll  ·  /quit to exit")
	return asciiCat() + "\n\n" + title + "\n" + hint
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// Run starts the bubbletea program in the alternate screen and blocks until the
// user quits.
func Run(ctx context.Context, be Backend, mode string) error {
	p := tea.NewProgram(New(ctx, be, mode), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
