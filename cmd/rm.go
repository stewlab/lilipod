// Package cmd contains all the cobra commands for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/89luca89/lilipod/pkg/containerutils"
	"github.com/89luca89/lilipod/pkg/fileutils"
	"github.com/89luca89/lilipod/pkg/logging"
	"github.com/89luca89/lilipod/pkg/procutils"
	"github.com/89luca89/lilipod/pkg/utils"
	"github.com/spf13/cobra"
)

// NewRmCommand removes one or more containers from the host.
func NewRmCommand() *cobra.Command {
	rmCommand := &cobra.Command{
		Use:              "rm [flags] IMAGE",
		Short:            "Remove one or more containers",
		PreRunE:          logging.Init,
		RunE:             rm,
		SilenceUsage:     true,
		SilenceErrors:    false,
		TraverseChildren: true,
	}

	rmCommand.Flags().SetInterspersed(false)
	rmCommand.Flags().BoolP("force", "f", false, "force remove container")
	rmCommand.Flags().BoolP("all", "a", false, "remove all containers")
	rmCommand.Flags().BoolP("help", "h", false, "show help")
	rmCommand.Flags().String("log-level", "", "log messages above specified level")

	return rmCommand
}

func rm(cmd *cobra.Command, arguments []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	delAll, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	if len(arguments) < 1 && !delAll {
		return cmd.Help()
	}

	// if we want to delete all, just get a list of the targets and add it to
	// the arguments.
	if delAll {
		arguments = []string{}

		containers, err := os.ReadDir(containerutils.ContainerDir)
		if err != nil {
			return fmt.Errorf("no containers found")
		}

		for _, i := range containers {
			arguments = append(arguments, i.Name())
		}
	}

	if force {
		for _, container := range arguments {
			if containerutils.IsRunning(container) {
				_ = containerutils.Stop(container, true, 5)
				for i := 0; i < 50; i++ {
					if !containerutils.IsRunning(container) {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
		}
	}

	success, err := procutils.EnsureFakeRoot(true)
	if err != nil {
		return err
	}

	if success {
		return nil
	}

	for _, container := range arguments {
		if containerutils.IsRunning(container) {
			if force {
				_ = containerutils.Stop(container, true, 5)
				for i := 0; i < 50; i++ {
					if !containerutils.IsRunning(container) {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
			if containerutils.IsRunning(container) {
				return fmt.Errorf("cannot remove container %s, as it is running", container)
			}
		}

		targetDIR := filepath.Join(containerutils.ContainerDir, container)
		if !fileutils.Exist(targetDIR) {
			targetDIR = containerutils.GetDir(container)
		}

		// delete the targets.
		if fileutils.Exist(targetDIR) {
			// Simple retry logic for unmount
			err := fileutils.Umount(filepath.Join(targetDIR, "rootfs"))
			if err != nil {
				return err
			}

			logging.LogDebug("deleting: %s in %s", container, targetDIR)

			err = os.RemoveAll(targetDIR)
			if err != nil {
				return err
			}

			err = os.RemoveAll(
				filepath.Join(utils.GetLilipodHome(), "volumes", containerutils.GetID(container)),
			)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("container %s does not exist", container)
		}

		fmt.Println(container)
	}

	return nil
}
