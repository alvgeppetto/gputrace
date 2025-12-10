package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tmc/gputrace/cmd/gputrace/cmd/serve"
)

var serveCmd = &cobra.Command{
	Use:   "serve <trace.gputrace>",
	Short: "Start a web server to browse the trace",
	Args:  cobra.ExactArgs(1),
	RunE:  runServe,
}

var servePort int

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "Port to serve on")
}

func runServe(cmd *cobra.Command, args []string) error {
	tracePath := args[0]
	if err := checkTraceFile(tracePath); err != nil {
		return err
	}

	fmt.Printf("Serving trace at http://localhost:%d\n", servePort)
	fmt.Println("Press Ctrl+C to stop")

	return serve.StartServer(tracePath, servePort)
}
