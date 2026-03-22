package cmd

import (
	"fmt"
	"os"

	"github.com/cmyster/fakeoid/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start server and send a test prompt",
	Long:  "Starts llama-server (via root command lifecycle), sends a streaming test prompt, and prints tokens.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := server.NewClient(srvPort, 0, 0)
		messages := []server.Message{
			{Role: "user", Content: "Say hello in one sentence."},
		}

		_, err := client.StreamChatCompletion(cmd.Context(), messages, func(token string) {
			fmt.Print(token)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: streaming failed: %s\n", err)
			return err
		}
		fmt.Println()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
