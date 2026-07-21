package components

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CooDdk/freexnats/internal/config"
	"github.com/CooDdk/freexnats/internal/ui"
)

type tickMsg time.Time

type SplashScreen struct {
	progress  float64
	status    string
	done      bool
	startTime time.Time
}

func NewSplashScreen() *SplashScreen {
	return &SplashScreen{
		progress:  0,
		status:    "Initializing...",
		startTime: time.Now(),
	}
}

func (s *SplashScreen) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		s.animateProgress(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (s *SplashScreen) animateProgress() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Millisecond * 80)
		return progressTickMsg{}
	}
}

type progressTickMsg struct{}

func (s *SplashScreen) Update(msg tea.Msg) (*SplashScreen, tea.Cmd) {
	switch msg.(type) {
	case tickMsg:
		if s.progress < 100 {
			return s, tea.Batch(tick(), s.animateProgress())
		}
	case progressTickMsg:
		if s.progress < 100 {
			s.progress += 2
			if s.progress > 100 {
				s.progress = 100
			}
			s.updateStatus()
		}
		if s.progress >= 100 && !s.done {
			s.done = true
			s.status = "Ready!"
			return s, func() tea.Msg {
				time.Sleep(300 * time.Millisecond)
				return SplashDoneMsg{}
			}
		}
	case tea.KeyMsg:
		s.done = true
		return s, func() tea.Msg {
			return SplashDoneMsg{}
		}
	}
	return s, nil
}

func (s *SplashScreen) updateStatus() {
	switch {
	case s.progress < 20:
		s.status = "Loading modules..."
	case s.progress < 40:
		s.status = "Initializing UI..."
	case s.progress < 60:
		s.status = "Connecting to NATS..."
	case s.progress < 80:
		s.status = "Loading streams..."
	case s.progress < 100:
		s.status = "Almost there..."
	}
}

func (s *SplashScreen) View(width, height int) string {
	logo := PixelLogo()
	brandLine := lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render("──  "),
		BrandTitle(),
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render("  ·  "),
		lipgloss.NewStyle().Foreground(ui.BrandAccent).Bold(true).Render(config.AppVersion),
		lipgloss.NewStyle().Foreground(ui.LogoTrimDim).Render(" ──"),
	)

	subtitle := lipgloss.NewStyle().
		Foreground(ui.SubtleColor).
		Render(config.AppDesc)

	progressBar := s.renderProgressBar()

	statusText := lipgloss.NewStyle().
		Foreground(ui.TextColor).
		Render(s.status)

	hint := lipgloss.NewStyle().
		Foreground(ui.MutedColor).
		Faint(true).
		Render("Press any key to skip")

	rule := decorativeRule(maxLineWidth(logo))

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		rule,
		logo,
		rule,
		"",
		brandLine,
		subtitle,
		"",
		"",
		statusText,
		progressBar,
		"",
		hint,
	)

	verticalPad := (height - lipgloss.Height(content)) / 2
	if verticalPad < 0 {
		verticalPad = 0
	}

	topPad := lipgloss.NewStyle().Height(verticalPad).Render("")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Center, topPad, content),
	)
}

func (s *SplashScreen) renderProgressBar() string {
	barWidth := 40
	filled := int(s.progress / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	filledStyle := lipgloss.NewStyle().
		Foreground(ui.Accent)

	emptyStyle := lipgloss.NewStyle().
		Foreground(ui.BgLighter)

	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += filledStyle.Render("#")
		} else {
			bar += emptyStyle.Render("-")
		}
	}

	percent := lipgloss.NewStyle().
		Foreground(ui.PrimaryLight).
		Bold(true).
		Render(" " + formatPercent(s.progress) + " ")

	return bar + percent
}

func formatPercent(p float64) string {
	if p >= 100 {
		return "100%"
	}
	return formatFloat(p) + "%"
}

func formatFloat(f float64) string {
	if f < 10 {
		return " " + floatToString(f)
	}
	return floatToString(f)
}

func floatToString(f float64) string {
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	if frac == 0 {
		return itoa(whole)
	}
	return itoa(whole) + "." + itoa(frac)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

type SplashDoneMsg struct{}
