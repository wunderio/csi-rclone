package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/li-il-li/csi-rclone/pkg/kube"
	"github.com/li-il-li/csi-rclone/pkg/rclone"
	"github.com/spf13/cobra"
)

var (
	endpoint string
	nodeID   string
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {

	cmd := &cobra.Command{
		Use:   "rclone",
		Short: "CSI based rclone driver",
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the CSI driver.",
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}
	cmd.AddCommand(runCmd)

	runCmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "node id")
	runCmd.MarkPersistentFlagRequired("nodeid")

	runCmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "CSI endpoint")
	runCmd.MarkPersistentFlagRequired("endpoint")

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

	cmd.ParseFlags(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	kubeClient, err := kube.GetK8sClient()
	if err != nil {
		panic(err)
	}
	d := rclone.NewDriver(nodeID, endpoint, kubeClient)
	d.Run()
}
