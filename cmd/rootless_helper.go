// Package cmd contains all the cobra commands for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/stewlab/lilipod/pkg/constants"
	"github.com/stewlab/lilipod/pkg/fileutils"
	"github.com/stewlab/lilipod/pkg/logging"
	"github.com/stewlab/lilipod/pkg/procutils"
	"github.com/spf13/cobra"
)

// EnvKey is the environment variable name that we set to distinguish
// if we're the parent or child during this operation.
const EnvKey = "ROOTLESS_HELPER_CHILD"

// NewRootlessHelperCommand will delete an OCI image in the configured DIR.
func NewRootlessHelperCommand() *cobra.Command {
	rootlessHelperCommand := &cobra.Command{
		Use:              "rootless-helper [command]",
		Short:            "Execute command as fake root",
		Hidden:           true,
		PreRunE:          logging.Init,
		RunE:             rootlessHelper,
		SilenceErrors:    false,
		SilenceUsage:     true,
		TraverseChildren: true,
	}

	rootlessHelperCommand.Flags().SetInterspersed(false)
	rootlessHelperCommand.Flags().String("log-level", "", "log messages above specified level")

	return rootlessHelperCommand
}

// rootlessHelper will either start as parend or as child depending
// on the content of environment EnvKey.
func rootlessHelper(_ *cobra.Command, arguments []string) error {
	if os.Getenv(EnvKey) == constants.TrueString {
		return child(arguments)
	}

	return parent(arguments)
}

// parent will relaunch the rootless-helper in child mode.
// child process will be executed in a new user namespace, and
// newuidmap/newgidmap will be used to write a new user/group-mapping
// inside it, in order to be fake-root in that namespace, but with
// other groups and users mapped.
// parent will wait for child's SIGCHLD signal before starting the mapping.
func parent(arguments []string) error {
	logging.LogDebug("parent: preparing to fork child")

	childReady := make(chan os.Signal, 1)
	signal.Notify(childReady, syscall.SIGCHLD)
	defer signal.Stop(childReady)

	childArgs := []string{"rootless-helper"}
	if lvl := logging.GetLogLevel(); lvl != "" {
		childArgs = append(childArgs, "--log-level", lvl)
	}
	childArgs = append(childArgs, arguments...)

	cmd := exec.Command(os.Args[0], childArgs...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, EnvKey+"=true")

	logging.LogDebug("parent: executing child: %v", cmd.Args)

	err := cmd.Start()
	if err != nil {
		return err
	}

	logging.LogDebug("parent: waiting for child to start")

	<-childReady

	logging.LogDebug("parent: child is ready")
	logging.LogDebug("parent: setting child uid/gid mappings")

	// set newuidmap
	err = setupUIDGIDMap(cmd.Process.Pid)
	if err != nil {
		return err
	}

	logging.LogDebug("parent: waiting for child completion")

	return cmd.Wait()
}

// child is launched by parent and will wait until the uid/gid-mapping is performed.
// after that it will execve the input arguments.
func child(arguments []string) error {
	logging.LogDebug("child: we're the child, notify the parent that we're ready")

	err := syscall.Kill(os.Getppid(), syscall.SIGCHLD)
	if err != nil {
		return err
	}

	logging.LogDebug("child: waiting for uid/gid map to be complete")
	// we're child, wait for GID to become 0
	deadline := time.Now().Add(5 * time.Second)
	for {
		if os.Getuid() == 0 && os.Getegid() == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for uid/gid mapping (uid=%d gid=%d)", os.Getuid(), os.Getegid())
		}
		time.Sleep(time.Millisecond * 5)
	}

	logging.LogDebug("child: now we're fake root")

	command, err := exec.LookPath(arguments[0])
	if err != nil {
		command = arguments[0]
	}

	logging.LogDebug("child: execve the input command: %v", arguments)

	return syscall.Exec(command, arguments, os.Environ())
}

// setupUIDGIDMap will perform the uid/gid mapping of input pid process using
// newuidmap/newgidmap. For this to work only the ppid of the input pid can
// perform this action, so we perform it in the parent process.
func setupUIDGIDMap(pid int) error {
	logging.LogDebug("getting user subuid/subgid ranges")

	uMaps, gMaps, err := procutils.GetSubIDRanges()
	if err == nil && len(uMaps) >= 3 && len(gMaps) >= 3 {
		if _, lookErr := exec.LookPath("newuidmap"); lookErr == nil {
			uidMap := []string{
				strconv.Itoa(pid),
				"0",
				uMaps[0],
				"1",
				"1",
				uMaps[1],
				uMaps[2],
			}
			gidMap := []string{
				strconv.Itoa(pid),
				"0",
				gMaps[0],
				"1",
				"1",
				gMaps[1],
				gMaps[2],
			}

			logging.LogDebug("user subuid map is %v", uidMap)
			logging.LogDebug("user subgid map is %v", gidMap)

			cmd := exec.Command("newuidmap", uidMap...)
			if out, err := cmd.CombinedOutput(); err == nil {
				cmdGid := exec.Command("newgidmap", gidMap...)
				outGid, errGid := cmdGid.CombinedOutput()
				if errGid == nil {
					return nil
				}
				logging.LogDebug("newgidmap failed: %s, falling back to direct proc mapping", string(outGid))
			} else {
				logging.LogDebug("newuidmap failed: %s, falling back to direct proc mapping", string(out))
			}
		}
	}

	// Fallback: direct unprivileged single-user mapping (0 -> current user)
	logging.LogDebug("using direct /proc uid/gid mapping")
	uid := strconv.Itoa(os.Getuid())
	gid := strconv.Itoa(os.Getgid())
	_ = fileutils.WriteFile(fmt.Sprintf("/proc/%d/setgroups", pid), []byte("deny"), 0o644)
	_ = fileutils.WriteFile(fmt.Sprintf("/proc/%d/uid_map", pid), []byte(fmt.Sprintf("0 %s 1\n", uid)), 0o644)
	return fileutils.WriteFile(fmt.Sprintf("/proc/%d/gid_map", pid), []byte(fmt.Sprintf("0 %s 1\n", gid)), 0o644)
}
