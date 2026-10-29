// Package cmd contains all the cobra commands for the CLI application.
package cmd

import (
	"fmt"
	"os"

	"github.com/89luca89/lilipod/pkg/containerutils"
	"github.com/89luca89/lilipod/pkg/logging"
	"github.com/89luca89/lilipod/pkg/utils"
	"github.com/spf13/cobra"
)

// NewEnterCommand will enter target container environment.
func NewEnterCommand() *cobra.Command {
	enterCommand := &cobra.Command{
		Use:              "enter [command]",
		Short:            "Execute command as fake root",
		Hidden:           true,
		PreRunE:          logging.Init,
		RunE:             enterContainer,
		SilenceErrors:    false,
		SilenceUsage:     true,
		TraverseChildren: true,
	}

	enterCommand.Flags().SetInterspersed(false)
	enterCommand.Flags().Bool("tty", false, "")
	enterCommand.Flags().String("config", "", "")
	enterCommand.Flags().String("log-level", "", "log messages above specified level")

	return enterCommand
}

func enterContainer(cmd *cobra.Command, _ []string) error {
	config, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}

	tty, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}

	conf, err := utils.InitConfig([]byte(config))
	if err != nil {
		logging.LogError("invalid config: %+v", err)

		return fmt.Errorf("invalid config: %+w", err)
	}

	err = containerutils.RunContainer(tty, conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enterContainer error: %+v\n", err)
		return err
	}
	return nil
}
