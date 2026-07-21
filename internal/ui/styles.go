package ui

import "github.com/charmbracelet/lipgloss"

var (
	BgColor      = lipgloss.Color("#0F172A")
	BgLightColor = lipgloss.Color("#1E293B")
	BgLighter    = lipgloss.Color("#334155")
	BorderColor  = lipgloss.Color("#475569")
	TextColor    = lipgloss.Color("#E2E8F0")
	TextMuted    = lipgloss.Color("#94A3B8")
	TextFaint    = lipgloss.Color("#64748B")

	Primary      = lipgloss.Color("#3B82F6")
	PrimaryLight = lipgloss.Color("#60A5FA")
	Accent       = lipgloss.Color("#22D3EE")
	Success      = lipgloss.Color("#10B981")
	Warning      = lipgloss.Color("#F59E0B")
	Error        = lipgloss.Color("#EF4444")

	SelectionBg = lipgloss.Color("#1E40AF")
	SelectionFg = lipgloss.Color("#FFFFFF")

	LogoColor1  = lipgloss.Color("#06B6D4")
	LogoColor2  = lipgloss.Color("#22D3EE")
	LogoColor3  = lipgloss.Color("#A855F7")
	LogoColor4  = lipgloss.Color("#C084FC")
	LogoTrim    = lipgloss.Color("#CA8A04")
	LogoTrimDim = lipgloss.Color("#854D0E")

	BrandPrimary   = Primary
	BrandSecondary = PrimaryLight
	BrandAccent    = Accent
	SubtleColor    = TextMuted
	MutedColor     = TextFaint
	WarningColor   = Warning
	ErrorColor     = Error
	SuccessColor   = Success
	SelectedColor  = SelectionBg
)

var (
	PagePadding = 2
)

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(TextColor)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(TextMuted)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Background(BgLightColor)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(SelectionFg).
				Background(SelectionBg).
				Bold(true)

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(TextColor)

	DimItemStyle = lipgloss.NewStyle().
			Foreground(TextMuted)
)
