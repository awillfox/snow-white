package main

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

func main() {
	// Placeholder to ensure dependencies are direct requires
	_ = bubbletea.Model(nil)
	_ = huh.NewForm()
	_ = spinner.New()
	_ = lipgloss.NewStyle()
	_ = pgx.Connect
	_ = yaml.Node{}
}
