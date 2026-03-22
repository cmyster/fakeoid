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

		_, _, exists, err := model.CachedModelInfo("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}

		if exists && !force {
			match, err := model.VerifySHA256(model.DefaultModelPath(), model.DefaultModelHash)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				os.Exit(1)
			}
			if match {
				name, sizeBytes, _, _ := model.CachedModelInfo("")
				sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
				fmt.Printf("Model verified: %s (%.1fGB)\n", name, sizeGB)
				return nil
			}
			fmt.Fprintln(os.Stderr, "Model corrupt, re-downloading...")
		}

		if err := model.DownloadDefault(os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Download complete: %s\n", model.DefaultModelName)
		return nil
	},
}
