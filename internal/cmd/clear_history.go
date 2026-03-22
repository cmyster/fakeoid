package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmyster/fakeoid/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(clearHistoryCmd)
}

var clearHistoryCmd = &cobra.Command{
	Use:   "clear-history",
	Short: "Delete all task history from .fakeoid/",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %s", err)
		}
		if err := state.ClearHistory(filepath.Join(cwd, ".fakeoid")); err != nil {
			return fmt.Errorf("clear history: %s", err)
		}
		fmt.Fprintln(os.Stdout, "History cleared.")
		return nil
	},
}
