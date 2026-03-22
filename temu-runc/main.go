package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	// MUST match your go.mod module path
	"github/Lewin-B/temu-runc/utils"
)

func main() {
	app := &cli.Command{
		Name:  "temu-runc",
		Usage: "Minimal container runtime",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "create <container-id>",
				ArgsUsage: "<container-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					containerID := cmd.Args().Get(0)
					if containerID == "" {
						return fmt.Errorf("missing container id\nusage: temu-runc create <container-id>")
					}

					if _, err := utils.NewContainer(containerID); err != nil {
						return err
					}

					fmt.Printf("Container created: %s\n", containerID)
					return nil
				},
			},
			// chroot and mount command
			{
				Name:      "execute",
				Usage:     "execute <container-id>",
				ArgsUsage: "<container-id>",
				Hidden:    true,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					containerID := cmd.Args().Get(0)
					if containerID == "" {
						return fmt.Errorf("missing container id\nusage: temu-runc create <container-id>")
					}

					if err := utils.Executor(containerID); err != nil {
						return err
					}

					fmt.Println("Executor process ran succesfully")
					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
