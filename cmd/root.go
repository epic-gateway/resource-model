package cmd

import (
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	debug       bool
	metricsAddr string

	Root = &cobra.Command{
		Use:               "manager",
		Short:             "EPIC - Simplifying k8s Edge Access",
		PersistentPreRunE: configureLogging,
		Version:           version,
	}

	// version is set by the Makefile.
	version string = "development"
)

func init() {
	Root.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "version: %s" .Version}}
`)

	// Global flags
	Root.PersistentFlags().BoolVar(&debug, "debug", true, "Enable debug logs")
	Root.PersistentFlags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address to which the metrics endpoint binds")
}

func configureLogging(cmd *cobra.Command, args []string) error {
	opts := zap.Options{Development: debug}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	return nil
}
