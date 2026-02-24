package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {

	cmd := &cli.Command{
		Name:  "container",
		Usage: "Container management menu",
		Action: func(context.Context, *cli.Command) error {
			log.Println("hello from urfave")
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal("Whoops")
	}
}
