package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	debug       bool
	metricsAddr string

	Root = &cobra.Command{
		Use:               "manager",
		Short:             "EPIC - Simplifying k8s Edge Access",
		PersistentPreRunE: configureCommand,
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
	Root.PersistentFlags().String("config", "", "Override config file name")
}

// configureCommand configures logging, and file/env input using
// viper.
func configureCommand(cmd *cobra.Command, args []string) error {
	// Configure logging.
	opts := zap.Options{Development: debug}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Configure Viper's environment file handling.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_")) // service-group -> SERVICE_GROUP
	viper.SetEnvPrefix("EPIC")                             // SERVICE_GROUP -> EPIC_SERVICE_GROUP
	viper.AutomaticEnv()

	// Process the config file, if available.
	if cfgFlag := cmd.Flags().Lookup("config"); cfgFlag != nil && cfgFlag.Changed {
		// If the user specified a config file name then use that
		viper.SetConfigFile(cfgFlag.Value.String())
	} else {
		// Look for the config file in a few standard places.
		viper.SetConfigName("manager")
		viper.AddConfigPath("/opt/acnodal/etc/")
		viper.AddConfigPath("$HOME/.config/acnodal/")
		viper.AddConfigPath("$HOME/.acnodal/")
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			ctrl.Log.Info("Config file not found")
		} else {
			ctrl.Log.Error(err, "Processing config file")
		}
	} else {
		ctrl.Log.Info("Config file", "fileName", viper.ConfigFileUsed())
	}

	// Bind each cobra flag to its associated viper configuration
	// (config file and environment variable). Inspired by
	// https://github.com/carolynvs/stingoftheviper.
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not
		// set and viper has a value.
		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})

	return nil
}
