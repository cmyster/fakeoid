package cmd

import (
	"fmt"
	"os"

	"github.com/cmyster/fakeoid/internal/model"
	"github.com/spf13/cobra"
)

func init() {
	downloadCmd.Flags().BoolP("force", "f", false, "Force re-download even if model exists")
	rootCmd.AddCommand(downloadCmd)
}

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download or verify the AI model",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		cfg, err := model.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		if err := cfg.ValidateModelIdentity(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}

		_, _, exists, err := model.CachedModelInfo(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}

		if exists && !force {
			hash := cfg.EffectiveModelHash()
			if hash != "" {
				match, err := model.VerifySHA256(cfg.EffectiveModelPath(), hash)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					os.Exit(1)
				}
				if match {
					name, sizeBytes, _, _ := model.CachedModelInfo(cfg)
					sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
					fmt.Printf("Model verified: %s (%.1fGB)\n", name, sizeGB)
					return nil
				}
				fmt.Fprintln(os.Stderr, "Model corrupt, re-downloading...")
			} else {
				name, sizeBytes, _, _ := model.CachedModelInfo(cfg)
				sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
				fmt.Printf("Model exists: %s (%.1fGB) (no hash configured, skipping verification)\n", name, sizeGB)
				return nil
			}
		}

		if err := model.DownloadModel(cfg, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Download complete: %s\n", cfg.EffectiveModelName())
		return nil
	},
}
