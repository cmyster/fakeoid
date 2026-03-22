package cmd

import (
	"fmt"
	"os"

	"github.com/cmyster/fakeoid/internal/model"
	"github.com/cmyster/fakeoid/internal/validate"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify system requirements",
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := validate.RunAll(&validate.ExecRunner{}, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		for _, r := range results {
			if !r.Passed {
				fmt.Fprintf(os.Stderr, "error: %s\n", r.Error)
				os.Exit(1)
			}
			fmt.Printf("  \u2713 %s: %s\n", r.Name, r.Detail)
		}
		fmt.Println("\nAll checks passed.")

		// Model status (info only, does not affect exit code)
		name, sizeBytes, exists, _ := model.CachedModelInfo("")
		if exists {
			sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
			fmt.Printf("  \u2713 Model: %s (%.1fGB)\n", name, sizeGB)
		} else {
			fmt.Printf("  \u2717 Model: not downloaded\n")
			fmt.Println("System ready. Run fakeoid to download model.")
		}
		return nil
	},
}
