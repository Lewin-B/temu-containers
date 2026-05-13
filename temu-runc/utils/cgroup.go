package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func createV2(config CgroupConfig) error {
	// create cgroup directory
	cgroupPath := filepath.Join(config.ParentPath, config.Name)
	if err := os.Mkdir(cgroupPath, 0755); err != nil {
		return fmt.Errorf("error creating cgroup directory: %w", err)
	}

	// enable controllers for cgroup
	if err := enableControllers(config.ParentPath, []string{"cpu", "memory", "pids"}); err != nil {
		return err
	}

	// configure cpu limits
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

	// Configure memory limits.
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

	// Configure PID limit.
	if config.Pids.Max != "" {
		if err := writeCgroupFile(cgroupPath, "pids.max", config.Pids.Max); err != nil {
			return err
		}
	}

	// Optional I/O limits.
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
