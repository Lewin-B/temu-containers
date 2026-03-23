package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const ROOT_FS = "/home/ubuntu/mycontainer/rootfs"
const linuxOPath = 0x200000

type Container struct {
	ContainerId string
}

type ContainerConfig struct {
	ContainerID string `json:"containerid"`
	PID         int    `json:"init_pid"` // host PID
}

func runtimeDirForHost(containerID string) string {
	if runtimeDir := os.Getenv("TEMU_RUNTIME_DIR"); runtimeDir != "" {
		return runtimeDir
	}

	return fmt.Sprintf("/run/user/%d/temu-runc/%s", os.Getuid(), containerID)
}

func (c Container) startContainer() int {
	return 0
}

func NewContainer(containerID string) (*Container, error) {
	uid := os.Getuid()
	gid := os.Getgid()

	runtimeDir := runtimeDirForHost(containerID)
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir runtime dir: %w", err)
	}

	fifoPath := filepath.Join(runtimeDir, "exec.fifo")
	_ = os.Remove(fifoPath) // cleanup from previous run
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return nil, fmt.Errorf("mkfifo: %w", err)
	}

	cmd := exec.Command(os.Args[0], "execute", containerID)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "TEMU_RUNTIME_DIR="+runtimeDir)

	cmd.SysProcAttr = &syscall.SysProcAttr{
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
		return nil, fmt.Errorf("start executor container init: %w", err)
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

func Executor(containerID string) error {
	runtimeDir := runtimeDirForHost(containerID)
	fifoPath := filepath.Join(runtimeDir, "exec.fifo")

	fifoFD, err := syscall.Open(fifoPath, linuxOPath|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open exec fifo: %w", err)
	}
	fifoFile := os.NewFile(uintptr(fifoFD), fifoPath)
	defer fifoFile.Close()

	// New file system virtualization
	if err := syscall.Chroot(ROOT_FS); err != nil {
		return fmt.Errorf("chroot error %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("Chdir error: %w", err)
	}

	// New mounts
	if err := syscall.Mount("tmpfs", "dev", "tmpfs", 0, ""); err != nil {
		return fmt.Errorf("tmpfs mount error: %w", err)
	}
	if err := syscall.Mount("proc", "proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("proc mount error: %w", err)
	}

	if err := syscall.Sethostname([]byte("temu-" + containerID)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	procFIFOPath := fmt.Sprintf("/proc/self/fd/%d", fifoFD)
	readFD, err := syscall.Open(procFIFOPath, syscall.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open exec fifo through procfd: %w", err)
	}
	readFile := os.NewFile(uintptr(readFD), procFIFOPath)
	defer readFile.Close()

	buf := make([]byte, 128)
	if _, err := readFile.Read(buf); err != nil {
		return fmt.Errorf("read start signal: %w", err)
	}

	if err := fifoFile.Close(); err != nil {
		return fmt.Errorf("close exec fifo pathfd: %w", err)
	}

	return syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ())

}

func Start(containerID string) error {
	runtimeDir := runtimeDirForHost(containerID)
	fifoPath := filepath.Join(runtimeDir, "exec.fifo")

	var fifo *os.File
	deadline := time.Now().Add(5 * time.Second)
	for {
		fd, err := syscall.Open(fifoPath, syscall.O_WRONLY|syscall.O_NONBLOCK, 0600)
		if err == nil {
			fifo = os.NewFile(uintptr(fd), fifoPath)
			break
		}
		if !errors.Is(err, syscall.ENXIO) {
			return fmt.Errorf("open exec fifo: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("open exec fifo: timed out waiting for container init to open read side")
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer fifo.Close()

	if _, err := fifo.WriteString("continue\n"); err != nil {
		return fmt.Errorf("write exec fifo: %w", err)
	}

	return nil
}
