package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ssh-portfolio/albumart"
	"ssh-portfolio/spotify"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
)

const (
	host = "0.0.0.0"
	port = "22"
)

var spotifyClient *spotify.Client

type tickMsg time.Time

type trackMsg struct {
	track *spotify.Track
	err   error
}

type model struct {
	width        int
	height       int
	currentTrack *spotify.Track
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchTrack,
		tickEvery(10*time.Second),
	)
}

func fetchTrack() tea.Msg {
	if spotifyClient == nil {
		return trackMsg{nil, nil}
	}

	track, err := spotifyClient.GetCurrentlyPlaying()
	if err != nil {
		return trackMsg{nil, err}
	}

	if track == nil {
		track, err = spotifyClient.GetRecentlyPlayed()
		if track != nil {
			track.IsPlaying = false
		}
	}

	return trackMsg{track, err}
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case trackMsg:
		if msg.err == nil && msg.track != nil {
			m.currentTrack = msg.track
		}
		return m, nil

	case tickMsg:
		return m, fetchTrack

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	// Cores principais
	spotifyGreen = lipgloss.Color("#1DB954")
	spotifyBlack = lipgloss.Color("#191414")
	subtleGray   = lipgloss.Color("#535353")
	lightGray    = lipgloss.Color("#B3B3B3")
	white        = lipgloss.Color("#FFFFFF")

	titleStyle = lipgloss.NewStyle().
			Foreground(spotifyGreen).
			Bold(true)

	trackNameStyle = lipgloss.NewStyle().
			Foreground(white).
			Bold(true)

	artistStyle = lipgloss.NewStyle().
			Foreground(lightGray)

	albumStyle = lipgloss.NewStyle().
			Foreground(subtleGray).
			Italic(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(subtleGray)

	widgetBorder = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(spotifyGreen).
			Padding(1, 2)

	emptyWidgetStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(subtleGray).
				Padding(1, 2).
				Foreground(subtleGray)
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		loadingStyle := lipgloss.NewStyle().
			Foreground(spotifyGreen).
			Bold(true)
		return loadingStyle.Render("● Carregando...")
	}

	spotifyWidget := m.renderSpotifyWidget()

	footer := footerStyle.Render(" Pressione q ou Enter para sair ")

	fullContent := lipgloss.JoinVertical(lipgloss.Center,
		spotifyWidget,
		footer,
	)

	contentHeight := lipgloss.Height(fullContent)
	topPadding := (m.height - contentHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	layout := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Top).
		PaddingTop(topPadding)

	return layout.Render(fullContent)
}

func (m model) renderSpotifyWidget() string {
	if m.currentTrack == nil {
		content := lipgloss.JoinVertical(lipgloss.Center,
			titleStyle.Render("♫ Spotify"),
			"",
			artistStyle.Render("Nenhuma música"),
		)
		return emptyWidgetStyle.Render(content)
	}

	art, _ := albumart.RenderFromURL(m.currentTrack.ArtworkURL, 16, 8)

	artFrame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(subtleGray).
		Render(art)

	trackName := m.currentTrack.Name
	if len(trackName) > 26 {
		trackName = trackName[:23] + "..."
	}

	artist := m.currentTrack.Artist
	if len(artist) > 26 {
		artist = artist[:23] + "..."
	}

	album := m.currentTrack.Album
	if len(album) > 26 {
		album = album[:23] + "..."
	}

	textContent := lipgloss.JoinVertical(lipgloss.Left,
		trackNameStyle.Render(trackName),
		artistStyle.Render(artist),
		albumStyle.Render(album),
	)

	textStyle := lipgloss.NewStyle().
		Width(28).
		PaddingLeft(2)

	content := lipgloss.JoinHorizontal(lipgloss.Center, artFrame, textStyle.Render(textContent))

	return widgetBorder.Render(content)
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()
	m := model{
		width:  pty.Window.Width,
		height: pty.Window.Height,
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

func main() {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	refreshToken := os.Getenv("SPOTIFY_REFRESH_TOKEN")

	if clientID != "" && clientSecret != "" && refreshToken != "" {
		spotifyClient = spotify.NewClient(clientID, clientSecret, refreshToken)
		log.Info("Spotify client initialized")
	} else {
		log.Warn("Spotify credentials not found, widget disabled")
	}

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
		),
	)
	if err != nil {
		log.Error("Erro ao criar servidor", "error", err)
		os.Exit(1)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Info("Servidor SSH iniciado", "host", host, "port", port)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Erro no servidor", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Encerrando servidor...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Erro ao encerrar servidor", "error", err)
	}
}
