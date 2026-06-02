package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mewcode/internal/agent"
	"mewcode/internal/permission"
)

// footerReserve is the number of terminal rows kept for the divider + input +
// hint (or the permission prompt) below the scrollable conversation viewport.
const footerReserve = 4

// Backend is the slice of the agent loop the UI needs. The agent satisfies it.
// Keeping it an interface lets the UI be driven by a spy in tests and keeps the
// UI from sharing mutable state with the loop — they talk only through Events
// and these calls.
type Backend interface {
	Events() <-chan agent.Event
	Start(ctx context.Context, input string)
	SubmitDecision(permission.UserDecision)
	SetMode(string)
}

// eventMsg wraps one agent event as a bubbletea message.
type eventMsg struct{ ev agent.Event }

// waitForEvent blocks on the next agent event and delivers it as a message. It
// is re-issued after every event so the stream is consumed continuously.
func waitForEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg { return eventMsg{ev: <-ch} }
}

type toolCard struct {
	name    string
	status  string // running | ok | error
	summary string
}

// Model is the bubbletea model: a transcript, a streaming reply, tool cards, an
// input box, and an optional permission prompt.
type Model struct {
	be   Backend
	ctx  context.Context
	mode string
	cwd  string // working directory tools operate in (shown in the footer)

	input  textinput.Model
	vp     viewport.Model // scrollable conversation area
	ready  bool           // viewport sized (set on first WindowSizeMsg)
	width  int
	height int

	transcript []string             // committed lines
	streaming  string               // assistant reply currently streaming
	cards      []*toolCard          // cards for the in-flight turn
	cardByID   map[string]*toolCard // id → card
	busy       bool

	pending *permission.PendingRequest // non-nil while awaiting a permission choice
}

// New builds the model. mode is the initial permission mode label (for display).
func New(ctx context.Context, be Backend, mode string) Model {
	in := textinput.New()
	in.Placeholder = "ask MewCode something… (/mode plan|default|auto, /quit)"
	in.Focus()
	in.Prompt = "› "
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "(unknown)"
	}
	return Model{
		be:         be,
		ctx:        ctx,
		mode:       mode,
		cwd:        cwd,
		input:      in,
		cardByID:   map[string]*toolCard{},
		transcript: []string{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, waitForEvent(m.be.Events()))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = msg.Width - 4
		h := msg.Height - footerReserve
		if h < 1 {
			h = 1
		}
		if m.ready {
			m.vp.Width, m.vp.Height = msg.Width, h
		} else {
			m.vp = viewport.New(msg.Width, h)
			m.ready = true
		}
		m.syncViewport()
		return m, nil

	case tea.MouseMsg: // wheel scroll
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() { // page keys scroll the transcript
		case "pgup", "pgdown", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)

	case eventMsg:
		m.applyEvent(msg.ev)
		m.syncViewport()
		return m, waitForEvent(m.be.Events())
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a permission prompt is up, keys pick an option instead of typing.
	if m.pending != nil {
		switch key.String() {
		case "a", "1":
			return m.resolvePermission(permission.AllowOnce), nil
		case "s", "2":
			return m.resolvePermission(permission.AlwaysAllow), nil
		case "d", "3", "esc":
			return m.resolvePermission(permission.Refuse), nil
		}
		return m, nil
	}

	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		return m.submitInput()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.busy {
		return m, nil
	}
	m.input.Reset()

	switch {
	case text == "/quit" || text == "/exit":
		return m, tea.Quit
	case strings.HasPrefix(text, "/mode "):
		mode := strings.TrimSpace(strings.TrimPrefix(text, "/mode "))
		switch mode {
		case "plan", "default", "auto":
			m.mode = mode
			m.be.SetMode(mode)
			m.transcript = append(m.transcript, dim("· mode → "+mode))
		default:
			m.transcript = append(m.transcript, errLine("unknown mode "+mode+" (plan|default|auto)"))
		}
		m.syncViewport()
		return m, nil
	}

	m.transcript = append(m.transcript, userStyle.Render("you")+"  "+text)
	m.busy = true
	m.cards = nil
	m.cardByID = map[string]*toolCard{}
	m.syncViewport()
	m.be.Start(m.ctx, text)
	return m, nil
}

func (m Model) resolvePermission(choice permission.Choice) Model {
	m.be.SubmitDecision(permission.UserDecision{ToolUseID: m.pending.ToolUseID, Choice: choice})
	m.pending = nil
	return m
}

// applyEvent folds one agent event into the model state.
func (m *Model) applyEvent(ev agent.Event) {
	switch e := ev.(type) {
	case agent.AssistantText:
		m.streaming += e.Text
	case agent.ToolStarted:
		c := &toolCard{name: e.Name, status: "running", summary: oneLine(e.Input)}
		m.cards = append(m.cards, c)
		m.cardByID[e.ID] = c
	case agent.ToolFinished:
		c := m.cardByID[e.ID]
		if c == nil { // no Started (e.g. denied in plan mode) — synthesize one
			c = &toolCard{name: e.Name}
			m.cards = append(m.cards, c)
			m.cardByID[e.ID] = c
		}
		if e.IsError {
			c.status = "error"
		} else {
			c.status = "ok"
		}
		c.summary = oneLine(e.Result)
	case agent.PermissionAsked:
		req := e.Request
		m.pending = &req
	case agent.TurnFinished:
		m.commitTurn()
	case agent.ErrorOccurred:
		m.transcript = append(m.transcript, errLine("error: "+e.Err.Error()))
		m.busy = false
		m.streaming = ""
	}
}

// commitTurn folds the streamed reply and tool cards into the transcript.
func (m *Model) commitTurn() {
	if m.streaming != "" {
		m.transcript = append(m.transcript, assistantStyle.Render("mew")+"  "+m.streaming)
	}
	for _, c := range m.cards {
		m.transcript = append(m.transcript, renderCard(c))
	}
	m.streaming = ""
	m.cards = nil
	m.cardByID = map[string]*toolCard{}
	m.busy = false
}

func (m Model) View() string {
	if !m.ready {
		return "starting MewCode…"
	}
	var b strings.Builder
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	b.WriteString(divider(m.width))
	if m.pending != nil {
		b.WriteString(renderPermission(m.pending))
	} else {
		b.WriteString(m.input.View())
		b.WriteString("\n")
		b.WriteString(dim(fmt.Sprintf("mode: %s   dir: %s   PgUp/PgDn scroll   ctrl+c quit", m.mode, m.cwd)))
	}
	return b.String()
}

// bodyContent is the full conversation (transcript + streaming reply + live
// cards), wrapped to the viewport width. The viewport handles overflow/scroll.
func (m Model) bodyContent() string {
	// The cat banner is pinned to the very top of the scrollback (Claude Code
	// style): conversation appends below it, and scrolling up reveals it again.
	lines := []string{catBanner(), ""}
	lines = append(lines, m.transcript...)
	if m.streaming != "" {
		lines = append(lines, assistantStyle.Render("mew")+"  "+m.streaming)
	}
	for _, c := range m.cards {
		lines = append(lines, renderCard(c))
	}
	return strings.Join(lines, "\n")
}

// syncViewport refreshes the viewport content, following new output to the
// bottom only when the user was already there (so scrolling up to read isn't
// interrupted).
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	wasBottom := m.vp.AtBottom()
	m.vp.SetContent(lipgloss.NewStyle().Width(m.vp.Width).Render(m.bodyContent()))
	if wasBottom {
		m.vp.GotoBottom()
	}
}
