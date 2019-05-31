package main

import (
	"flag"
	"fmt"
	"os"
	"github.com/spf13/cobra"
	"github.com/wunderio/csi-rclone/pkg/rclone"
)

var (
	endpoint string
	nodeID   string
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {

	flag.CommandLine.Parse([]string{})

	cmd := &cobra.Command{
		Use:   "rclone",
		Short: "CSI based rclone driver",
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	cmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "node id")
	cmd.MarkPersistentFlagRequired("nodeid")

	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "CSI endpoint")
	cmd.MarkPersistentFlagRequired("endpoint")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Prints information about this version of csi rclone plugin",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`csi-rclone plugin
Version:    %s
`, rclone.DriverVersion)
		},
	}

	cmd.AddCommand(versionCmd)
	versionCmd.ResetFlags()

	cmd.ParseFlags(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	d := rclone.NewDriver(nodeID, endpoint)
	d.Run()
}
