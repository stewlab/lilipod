// Package procutils contains helpers and utilities for managing processes.
package procutils

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stewlab/lilipod/pkg/constants"
	"github.com/stewlab/lilipod/pkg/logging"
)

// EnsureFakeRoot will ensure process is executed with rootless-helper.
func EnsureFakeRoot(interactive bool) (bool, error) {
	logging.LogDebug("ensuring we're either root or fake-root")

	// As files can be owned by root or fake roots, let's run either as root or
	// with rootless-helper
	if os.Getuid() == 0 &&
		(os.Getenv("ROOTFUL") != constants.TrueString ||
			os.Getenv("UNSHARED") == constants.TrueString) {
		logging.LogDebug("we're 0:0")
		logging.LogDebug(os.Getenv("ROOTFUL"))
		logging.LogDebug(os.Getenv("UNSHARED"))

		return false, nil
	}

	var cmd *exec.Cmd

	// if we're running as proper root, let's do a mount unshare and continue
	if os.Getuid() == 0 &&
		os.Getenv("ROOTFUL") == constants.TrueString &&
		os.Getenv("UNSHARED") != constants.TrueString {
		cmd = exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "ROOTFUL=true")
		cmd.Env = append(cmd.Env, "UNSHARED=true")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential:                 &syscall.Credential{Uid: 0, Gid: 0},
			Cloneflags:                 syscall.CLONE_NEWNS,
			GidMappingsEnableSetgroups: true,

			Setsid:     true,
			Foreground: false,
			Pdeathsig:  syscall.SIGTERM,
		}
	} else {
		userMap, gidMap, err := GetSubIDRanges()
		if err != nil {
			logging.LogDebug("error: %+v", err)

			return false, err
		}

		args := []string{"rootless-helper", "--log-level", logging.GetLogLevel()}
		args = append(args, os.Args...)
		cmd = exec.Command(os.Args[0], args...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "ROOTFUL=false")
		cmd.Env = append(cmd.Env, "PARENT_UID_MAP="+strings.Join(userMap, ":"))
		cmd.Env = append(cmd.Env, "PARENT_GID_MAP="+strings.Join(gidMap, ":"))
	}

	logging.LogDebug("executing %v", cmd.Args)

	if interactive {
		err := RunWithTTY(cmd)
		if err != nil {
			logging.LogDebug("error: %+v", err)

			return false, err
		}

		return true, nil
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// this is needed to completely detach a process
	cmd.SysProcAttr.Foreground = false
	cmd.SysProcAttr.Setsid = true

	logging.LogDebug("tty not specified, using cmd.Start")

	err := cmd.Start()
	if err != nil {
		logging.LogDebug("error: %+v", err)

		return false, err
	}

	logging.LogDebug("tty not specified, waiting for child to start")
	time.Sleep(time.Millisecond * 250)

	logging.LogDebug("tty not specified, releasing child")

	err = cmd.Process.Release()
	if err != nil {
		logging.LogDebug("error: %+v", err)

		return false, err
	}

	return true, nil
}

// GetUIDGID will return a couple of uid/gid integers for input user.
// Input user can be in the form of username:group, or uid:gid or a mix of that.
func GetUIDGID(username string) (int, int) {
	logging.LogDebug("getting uid and gid for %s", username)

	groupname := username

	// check if we're in the format user:group or 1234:1234
	if strings.Contains(username, ":") {
		logging.LogDebug("splitting %s by ':'", username)

		split := strings.Split(username, ":")
		username = split[0]
		groupname = split[1]

		logging.LogDebug("username=%s groupname=%s", username, groupname)
	}

	logging.LogDebug("checking if input username is numeric")
	// if we're passed numeric IDs, just convert and return
	uid, err := strconv.ParseInt(username, 10, 32)
	if err != nil {
		uid = -1
	}

	logging.LogDebug("checking if input groupname is numeric")
	// if we're passed numeric IDs, just convert and return
	gid, err := strconv.ParseInt(groupname, 10, 32)
	if err != nil {
		gid = -1
	}

	// return converted IDs
	if uid > 0 && gid > 0 {
		logging.LogDebug("input username and groupname were numeric, returning %d %d", uid, gid)

		return int(uid), int(gid)
	}

	logging.LogDebug("input username and groupname were not numeric, looking up")

	// if we're here it is because they were not numbers
	// so we'll need to lookup them
	user, err := user.Lookup(username)
	if err != nil {
		return 0, 0
	}

	logging.LogDebug("parsing looked up username uid and gid")

	// if we're passed numeric IDs, just convert and return
	uid, err = strconv.ParseInt(user.Uid, 10, 32)
	if err != nil {
		uid = -1
	}

	gid, err = strconv.ParseInt(user.Gid, 10, 32)
	if err != nil {
		gid = -1
	}

	// return converted IDs
	if uid > 0 && gid > 0 {
		logging.LogDebug("uid and gid parsed successfully, returning %d %s", uid, gid)

		return int(uid), int(gid)
	}

	// default to root
	logging.LogDebug("parsing looked up uid/gid failed, defaulting to root 0:0")

	return 0, 0
}

// GetSubIDRanges will return a slice of subUIDs and subGIDs for
// running user.
// This function will use the "getsubids" program to discover them.
func GetSubIDRanges() ([]string, []string, error) {
	user, err := user.Current()
	if err != nil {
		logging.LogError("%v", err)

		return nil, nil, err
	}

	subUIDout, err := exec.Command("getsubids", user.Username).Output()
	if err != nil {
		logging.LogError("%v", err)

		return nil, nil, err
	}

	subUIDSlice := strings.Split(
		strings.Trim(string(subUIDout), "\n"),
		" ")[2:]

	subGIDout, err := exec.Command("getsubids", "-g", user.Username).Output()
	if err != nil {
		logging.LogError("%v", err)

		return nil, nil, err
	}

	subGIDSlice := strings.Split(
		strings.Trim(string(subGIDout), "\n"),
		" ")[2:]

	subUIDSlice = append([]string{user.Uid}, subUIDSlice...)
	subGIDSlice = append([]string{user.Gid}, subGIDSlice...)

	return subUIDSlice, subGIDSlice, nil
}

// SetProcessKeepIDMaps will set child process uid/gid mappings.
func SetProcessKeepIDMaps(cmd *exec.Cmd, uidMap, gidMap string) error {
	uids := strings.Split(uidMap, ":")[0]
	uidsizes := strings.Split(uidMap, ":")[2]
	gids := strings.Split(gidMap, ":")[0]
	gidsizes := strings.Split(gidMap, ":")[2]

	uid, err := strconv.ParseInt(uids, 10, 32)
	if err != nil {
		logging.LogError("%v", err)

		return err
	}

	gid, err := strconv.ParseInt(gids, 10, 32)
	if err != nil {
		logging.LogError("%v", err)

		return err
	}

	uidsize, err := strconv.ParseInt(uidsizes, 10, 32)
	if err != nil {
		logging.LogError("%v", err)

		return err
	}

	gidsize, err := strconv.ParseInt(gidsizes, 10, 32)
	if err != nil {
		logging.LogError("%v", err)

		return err
	}

	cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{
		{
			ContainerID: 0,
			HostID:      1,
			Size:        int(uid),
		},
		{
			ContainerID: int(uid),
			HostID:      0,
			Size:        1,
		},
		{
			ContainerID: int(uid) + 1,
			HostID:      int(uid) + 1,
			Size:        int(uidsize - uid),
		},
	}
	cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{
		{
			ContainerID: 0,
			HostID:      1,
			Size:        int(gid),
		},
		{
			ContainerID: int(gid),
			HostID:      0,
			Size:        1,
		},
		{
			ContainerID: int(gid) + 1,
			HostID:      int(gid) + 1,
			Size:        int(gidsize - gid),
		},
	}

	logging.LogDebug("settings uidmap %v", cmd.SysProcAttr.UidMappings)
	logging.LogDebug("settings gidmap %v", cmd.SysProcAttr.GidMappings)

	return nil
}

// IsPidRunning will return whether or not the input pid is actually alive
// and not stopped or a zombie process.
func IsPidRunning(pid int) bool {
	processPath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")

	_, err := os.Stat(processPath)
	if err != nil {
		return false
	}

	out, err := os.ReadFile(processPath)
	if err != nil {
		return false
	}

	return len(out) > 0
}

// RunWithTTY will run input cmd using main process' stdin/out/err.
func RunWithTTY(cmd *exec.Cmd) error {
	logging.LogDebug("tty specified, just use cmd.Run")

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunInteractive will run input cmd using main process' stdin, but
// pipe stdout/err to main.
//
// This usually is used in combination with the ptyAgent inside a container.
func RunInteractive(cmd *exec.Cmd) error {
	logging.LogDebug("interactive but no tty, setting up pipes")

	cmd.Stdin = os.Stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	// Non-blockingly echo command output to terminal
	go func() { _, _ = io.Copy(os.Stdout, stdout) }()
	go func() { _, _ = io.Copy(os.Stderr, stderr) }()

	return cmd.Wait()
}

// RunDetached will run input cmd in the background and redirect all outputs to logfile.
func RunDetached(cmd *exec.Cmd, logfile string) error {
	logging.LogDebug("starting process detached in background, redirecting output to %s", logfile)

	logFile, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Foreground = false
	if cmd.SysProcAttr.Cloneflags == 0 {
		cmd.SysProcAttr.Setsid = true
	}

	err = cmd.Start()
	if err != nil {
		_ = logFile.Close()
		return err
	}

	_ = logFile.Close()
	return cmd.Process.Release()
}
