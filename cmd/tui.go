package cmd

import (
	"github.com/mrbooshehri/qix-go/internal/storage"
	"github.com/mrbooshehri/qix-go/internal/tui"
	"github.com/spf13/cobra"
)

var tuiProject string

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive terminal interface",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(storage.Get(), tuiProject)
	},
}

func init() {
	tuiCmd.Flags().StringVarP(&tuiProject, "project", "p", "", "Open a project by name")
}
