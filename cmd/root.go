package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	debug       bool
	metricsAddr string

	rootCmd = &cobra.Command{
		Use:   "manager",
		Short: "EPIC - Simplifying k8s Edge Access",
	}
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logs")
	rootCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address to which the metrics endpoint binds")
}

// Execute runs the root command.
func Execute() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(debug)))

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
