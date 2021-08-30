package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	nodeId      string
	apiKey      string
	runAsDaemon bool
)

func waitForInterrupt() {
	interruptCh := make(chan os.Signal, 2)
	signal.Notify(interruptCh, os.Interrupt, syscall.SIGTERM)
	<-interruptCh
}

func main() {

	var rootCmd = &cobra.Command{
		Use: "pocketmon --node <your_node_id> --key <your_api_key>",
		Short: `Pocketmon monitors your pocket nodes and 
notifies you when there is an issue`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			go start(ctx, nodeId, apiKey, runAsDaemon)
			waitForInterrupt()
			cancel()
		},
	}

	rootCmd.Flags().StringVarP(&nodeId, "node", "n", "", "Node ID (required) (Get from lunar.dev)")
	rootCmd.MarkFlagRequired("node")
	rootCmd.Flags().StringVarP(&apiKey, "key", "k", "", "API Key (required) (Get from lunar.dev)")
	rootCmd.MarkFlagRequired("key")
	//@Todo
	// rootCmd.Flags().BoolVarP(&runAsDaemon, "daemon", "d", false, "Run in background as daemon")

	rootCmd.Execute()

}
