package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const ROOT_FS = "/home/ubuntu/mycontainer/rootfs"

type Container struct {
	ContainerId string
}

type ContainerConfig struct {
	ContainerID string `json:"containerid"`
	PID         int    `json:"init_pid"` // host PID
}

func (c Container) startContainer() int {
	return 0
}

func NewContainer(containerID string) (*Container, error) {
	uid := os.Getuid()
	gid := os.Getgid()

	runtimeDir := fmt.Sprintf("/run/user/%d/temu-runc/%s", uid, containerID)
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir runtime dir: %w", err)
	}

	fifoPath := filepath.Join(runtimeDir, "exec.fifo")
	_ = os.Remove(fifoPath) // cleanup from previous run
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return nil, fmt.Errorf("mkfifo: %w", err)
	}

	script := `
echo "[container] waiting for FIFO at: $FIFO_PATH" 1>&2

# Basic mounts
mount -t proc proc /proc

# /dev as tmpfs so shells/tools don't freak out
mount -t tmpfs tmpfs /dev

hostname "temu-` + containerID + `" 2>/dev/null || true

IFS= read -r msg < "$FIFO_PATH" || true

echo "[container] received: $msg" 1>&2
exec /bin/sh
`

	cmd := exec.Command("sh", "-c", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: ROOT_FS,
		// Dir will be set by cmd.Dir, but you can also chdir in the script.
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUSER,

		GidMappingsEnableSetgroups: false,

		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
		},
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start container init: %w", err)
	}

	containerCfg := ContainerConfig{
		ContainerID: containerID,
		PID:         cmd.Process.Pid,
	}

	data, err := json.MarshalIndent(containerCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	statePath := filepath.Join(runtimeDir, "state.json")
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return nil, fmt.Errorf("write state.json: %w", err)
	}

	return &Container{
		ContainerId: containerID,
	}, nil
}
