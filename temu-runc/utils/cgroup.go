package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
)

type CgroupConfig struct {
	ID string

	ParentPath string
	Name       string

	CPU    CPUConfig
	Memory MemoryConfig
	Pids   PidsConfig
	IO     *IOConfig
}

type CPUConfig struct {
	Max    string // cpu.max, example: "50000 100000" or "max 100000"
	Weight string // cpu.weight, example: "100"
}

type MemoryConfig struct {
	Max  string // memory.max, example: "256M" or "max"
	High string // memory.high, optional soft throttle limit
	Swap string // memory.swap.max, optional
}

type PidsConfig struct {
	Max string // pids.max
}

type IOConfig struct {
	Max string // io.max
}

type systemdProperty struct {
	Name  string
	Value dbus.Variant
}

type systemdAuxUnit struct {
	Name       string
	Properties []systemdProperty
}

func configureV2(cgroupPath string, config CgroupConfig) error {
	if config.CPU.Max != "" {
		if err := writeCgroupFile(cgroupPath, "cpu.max", config.CPU.Max); err != nil {
			return err
		}
	}

	if config.CPU.Weight != "" {
		if err := writeCgroupFile(cgroupPath, "cpu.weight", config.CPU.Weight); err != nil {
			return err
		}
	}

	if config.Memory.Max != "" {
		if err := writeCgroupFile(cgroupPath, "memory.max", config.Memory.Max); err != nil {
			return err
		}
	}

	if config.Memory.High != "" {
		if err := writeCgroupFile(cgroupPath, "memory.high", config.Memory.High); err != nil {
			return err
		}
	}

	if config.Memory.Swap != "" {
		if err := writeCgroupFile(cgroupPath, "memory.swap.max", config.Memory.Swap); err != nil {
			return err
		}
	}

	if config.Pids.Max != "" {
		if err := writeCgroupFile(cgroupPath, "pids.max", config.Pids.Max); err != nil {
			return err
		}
	}

	if config.IO != nil && config.IO.Max != "" {
		if err := writeCgroupFile(cgroupPath, "io.max", config.IO.Max); err != nil {
			return err
		}
	}

	return nil
}

func enableControllers(parentPath string, controllers []string) error {
	if len(controllers) == 0 {
		return nil
	}
	subtreePath := filepath.Join(parentPath, "cgroup.subtree_control")

	var values []string
	for _, controller := range controllers {
		values = append(values, "+"+controller)
	}

	content := strings.Join(values, " ")

	if err := os.WriteFile(subtreePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error enabling controllers: %w", err)
	}

	return nil

}

func writeCgroupFile(cgroupPath string, fileName string, value string) error {
	path := filepath.Join(cgroupPath, fileName)

	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("error writing %s: %w", path, err)
	}

	return nil
}

func CreateDelegatedUserScope(scopeName string, pid int) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connect to user dbus: %w", err)
	}
	defer conn.Close()

	props := []systemdProperty{
		{"Description", dbus.MakeVariant("temu container " + scopeName)},
		{"Delegate", dbus.MakeVariant(true)},
		{"PIDs", dbus.MakeVariant([]uint32{uint32(pid)})},
		{"Slice", dbus.MakeVariant("app.slice")},
	}

	aux := []systemdAuxUnit{}

	obj := conn.Object(
		"org.freedesktop.systemd1",
		"/org/freedesktop/systemd1",
	)

	call := obj.Call(
		"org.freedesktop.systemd1.Manager.StartTransientUnit",
		0,
		scopeName,
		"replace",
		props,
		aux,
	)
	if call.Err != nil {
		return fmt.Errorf("start transient scope %s: %w", scopeName, call.Err)
	}

	return nil
}

func AddProcess(cgroupPath string, pid int) error {
	procsPath := filepath.Join(cgroupPath, "cgroup.procs")
	return os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644)
}
