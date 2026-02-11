package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
)

const (
	host = "0.0.0.0"
	port = "2222"
)

type model struct{}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	return "Hello World\n\nPressione Enter (ou q) para sair.\n"
}

// teaHandler retorna um handler que cria uma nova instância do Bubble Tea para cada sessão SSH
func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	return model{}, []tea.ProgramOption{tea.WithAltScreen()}
}

func main() {
	// Cria o servidor SSH com o middleware do Bubble Tea
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

	// Canal para capturar sinais de encerramento
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Inicia o servidor em uma goroutine
	log.Info("Servidor SSH iniciado", "host", host, "port", port)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Erro no servidor", "error", err)
			done <- nil
		}
	}()

	// Aguarda sinal de encerramento
	<-done
	log.Info("Encerrando servidor...")

	// Graceful shutdown com timeout de 30 segundos
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Erro ao encerrar servidor", "error", err)
	}
}
