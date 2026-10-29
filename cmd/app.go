// Package cmd contains all the cobra commands for the CLI application.
package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/stewlab/lilipod/pkg/constants"
	"github.com/stewlab/lilipod/pkg/containerutils"
	"github.com/stewlab/lilipod/pkg/imageutils"
	"github.com/stewlab/lilipod/pkg/utils"
	"github.com/spf13/cobra"
)

// InitEnvironment initializes required lilipod directories, PATH, and dependency checks.
func InitEnvironment() error {
	_ = os.MkdirAll(utils.LilipodBinPath, 0o755)
	_ = os.MkdirAll(imageutils.ImageDir, 0o755)
	_ = os.MkdirAll(containerutils.ContainerDir, 0o755)

	path := utils.LilipodBinPath + ":" + os.Getenv("PATH")
	if err := os.Setenv("PATH", path); err != nil {
		return err
	}

	if os.Getenv("ROOTFUL") == "" {
		if err := os.Setenv("ROOTFUL", strconv.FormatBool(os.Getuid() == 0)); err != nil {
			return err
		}
	}

	return utils.EnsureUNIXDependencies(nil, nil)
}

// NewApp returns the root Cobra command for lilipod.
func NewApp() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:              "lilipod",
		Short:            "Manage containers and images",
		Version:          strings.TrimPrefix(constants.Version, "v"),
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true,
	}

	rootCmd.AddCommand(
		NewCpCommand(),
		NewCreateCommand(),
		NewEnterCommand(),
		NewExecCommand(),
		NewImagesCommand(),
		NewInspectCommand(),
		NewLogsCommand(),
		NewPsCommand(),
		NewPullCommand(),
		NewRenameCommand(),
		NewRmCommand(),
		NewRmiCommand(),
		NewRootlessHelperCommand(),
		NewRunCommand(),
		NewStartCommand(),
		NewStopCommand(),
		NewUpdateCommand(),
		NewVersionCommand(),
	)
	rootCmd.PersistentFlags().
		String("log-level", "", "log messages above specified level (debug, warn, warning, error)")

	return rootCmd
}
