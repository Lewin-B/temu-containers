package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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

func createCgroupPath(containerId string) (string, string) {
	uid := os.Getuid()
	cgroupName := fmt.Sprintf("container-%s.scope", containerId)
	cgroupParentPath := fmt.Sprintf(
		"/sys/fs/cgroup/user.slice/user-%d.slice/user@%d.service/app.slice",
		uid,
		uid,
	)

	return cgroupParentPath, cgroupName
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

	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
		dbus.WithMatchMember("JobRemoved"),
		dbus.WithMatchObjectPath("/org/freedesktop/systemd1"),
	); err != nil {
		return fmt.Errorf("subscribe to systemd job signals: %w", err)
	}
	defer conn.RemoveMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
		dbus.WithMatchMember("JobRemoved"),
		dbus.WithMatchObjectPath("/org/freedesktop/systemd1"),
	)

	var job dbus.ObjectPath
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
	if err := call.Store(&job); err != nil {
		return fmt.Errorf("read transient scope job path: %w", err)
	}

	return waitForSystemdJob(signals, job, scopeName, 5*time.Second)
}

func waitForSystemdJob(signals <-chan *dbus.Signal, job dbus.ObjectPath, unit string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case signal := <-signals:
			if signal == nil || signal.Name != "org.freedesktop.systemd1.Manager.JobRemoved" || len(signal.Body) < 4 {
				continue
			}

			removedJob, ok := signal.Body[1].(dbus.ObjectPath)
			if !ok || removedJob != job {
				continue
			}

			removedUnit, _ := signal.Body[2].(string)
			result, _ := signal.Body[3].(string)
			if result != "done" {
				return fmt.Errorf("systemd job for %s finished with result %q", removedUnit, result)
			}

			return nil
		case <-timer.C:
			return fmt.Errorf("timed out waiting for systemd job %s for %s", job, unit)
		}
	}
}

func FreezeUserUnit(unit string) error {
	return callUserSystemdManager("FreezeUnit", unit)
}

func ThawUserUnit(unit string) error {
	return callUserSystemdManager("ThawUnit", unit)
}

func KillUserUnit(unit string) error {
	return callUserSystemdManager("KillUnit", unit, "all", int32(syscall.SIGKILL))
}

func StopUserUnit(unit string) error {
	return callUserSystemdManager("StopUnit", unit, "replace")
}

func callUserSystemdManager(method string, args ...interface{}) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connect to user dbus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object(
		"org.freedesktop.systemd1",
		"/org/freedesktop/systemd1",
	)

	call := obj.Call(
		"org.freedesktop.systemd1.Manager."+method,
		0,
		args...,
	)
	if call.Err != nil {
		return fmt.Errorf("call %s: %w", method, call.Err)
	}

	return nil
}
