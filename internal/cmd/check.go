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
		cfg, cfgErr := model.LoadConfig()
		if cfgErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Config: %s\n", cfgErr)
		} else if err := cfg.ValidateModelIdentity(); err != nil {
			fmt.Printf("  ✗ Model: %s\n", err)
		} else {
			name, sizeBytes, exists, _ := model.CachedModelInfo(cfg)
			if exists {
				sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
				fmt.Printf("  ✓ Model: %s (%.1fGB)\n", name, sizeGB)
			} else {
				fmt.Printf("  ✗ Model: not downloaded\n")
				fmt.Println("System ready. Run fakeoid to download model.")
			}
		}
		return nil
	},
}
