package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/lunemis/mux/tmux"
	"github.com/lunemis/mux/ui"
)

var version = "dev"

// invertScrollFlag is the flag name shared by every command that has to pass
// the setting on to the mux that ends up drawing the list.
const invertScrollFlag = "invert-scroll"

const invertScrollHelp = "Invert mouse wheel direction (for natural scrolling)"

func main() {
	var invertScroll bool

	rootCmd := &cobra.Command{
		Use:     "mux",
		Short:   "TUI tmux session manager",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(invertScroll)
		},
		// Suppress cobra's default completion and help subcommands
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	rootCmd.SetVersionTemplate("mux {{.Version}}\n")
	rootCmd.Flags().BoolVar(&invertScroll, invertScrollFlag, false, invertScrollHelp)

	var popupInvertScroll bool
	popupCmd := &cobra.Command{
		Use:   "popup",
		Short: "Open mux as a tmux popup overlay",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tmux.OpenPopup(popupInvertScroll)
		},
	}
	popupCmd.Flags().BoolVar(&popupInvertScroll, invertScrollFlag, false, invertScrollHelp)

	var keybindInvertScroll bool
	setupKeybindCmd := &cobra.Command{
		Use:   "setup-keybind [key]",
		Short: fmt.Sprintf("Add popup keybinding to tmux config (default: %s)", tmux.DefaultBindKey),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := tmux.DefaultBindKey
			if len(args) > 0 {
				key = args[0]
			}
			return tmux.SetupKeybind(key, keybindInvertScroll)
		},
	}
	setupKeybindCmd.Flags().BoolVar(&keybindInvertScroll, invertScrollFlag, false,
		invertScrollHelp+" in the keybinding this writes")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show AI session summary for tmux statusbar",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}

	rootCmd.AddCommand(popupCmd, setupKeybindCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runStatus() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return err
	}

	var parts []string
	for _, s := range sessions {
		tool, ok := tmux.LookupAITool(s.ActiveCommand)
		if !ok {
			continue
		}
		parts = append(parts, tool.Icon)
	}

	if len(parts) == 0 {
		return nil // no AI sessions, output nothing
	}

	fmt.Print(fmt.Sprintf(" %s ", joinWith(parts, " ")))
	return nil
}

func joinWith(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

func runTUI(invertScroll bool) error {
	var opts []ui.Option
	if invertScroll {
		opts = append(opts, ui.WithInvertedScroll())
	}

	// WithMouseCellMotion turns on button-event tracking so the list responds to
	// clicks and the wheel. It also takes drag-to-select away from the terminal
	// while mux is up; hold shift to select text as before.
	p := tea.NewProgram(ui.NewModel(opts...), tea.WithAltScreen(), tea.WithMouseCellMotion())

	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	if m, ok := result.(ui.Model); ok {
		if name := m.AttachName(); name != "" {
			if err := ui.AttachToSession(name, m.AttachWindowIndex(), m.AttachPaneIndex()); err != nil {
				return fmt.Errorf("failed to attach: %w", err)
			}
		}
	}
	return nil
}
