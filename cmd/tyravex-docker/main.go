package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Tyravex/agent/internal/docker"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	runtime := docker.NewRuntime()

	switch args[0] {
	case "minecraft-install":
		fs := flag.NewFlagSet("minecraft-install", flag.ContinueOnError)

		id := fs.String("id", "mc-test", "server identifier")
		port := fs.Int("port", 25565, "host TCP port")
		memory := fs.Int64("memory", 2048, "container RAM in MiB")
		cpu := fs.Int64("cpu", 100, "CPU percentage (100 = one core)")
		image := fs.String(
			"image",
			"itzg/minecraft-server:latest",
			"Docker image",
		)

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected positional arguments")
		}
		if *port < 1 || *port > 65535 {
			return errors.New("port must be 1..65535")
		}
		if *memory < 1024 {
			return errors.New("Minecraft test requires at least 1024 MiB")
		}

		// Run this command only after accepting the Minecraft EULA.
		spec := docker.ServerSpec{
			ID:            *id,
			Image:         *image,
			MemoryMB:      *memory,
			CPUPercent:    *cpu,
			Port:          uint16(*port),
			ContainerPort: 25565,
			Env: map[string]string{
				"EULA":   "TRUE",
				"TYPE":   "PAPER",
				"MEMORY": strconv.FormatInt(*memory-256, 10) + "M",
			},
		}

		containerID, err := runtime.Install(ctx, spec)
		if err != nil {
			return err
		}

		fmt.Printf(
			"Created container %s for server %s.\n",
			containerID,
			*id,
		)
		return nil

	case "start", "stop", "restart", "status":
		if len(args) != 2 {
			return errors.New(
				"usage: tyravex-docker " + args[0] + " <server-id>",
			)
		}

		switch args[0] {
		case "start":
			return runtime.Start(ctx, args[1])
		case "stop":
			return runtime.Stop(ctx, args[1])
		case "restart":
			return runtime.Restart(ctx, args[1])
		default:
			status, err := runtime.Status(ctx, args[1])
			if err != nil {
				return err
			}
			fmt.Println(status)
			return nil
		}

	case "logs":
		fs := flag.NewFlagSet("logs", flag.ContinueOnError)
		lines := fs.Int(
			"lines",
			100,
			"number of recent lines (1..1000)",
		)

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New(
				"usage: tyravex-docker logs [-lines N] <server-id>",
			)
		}

		output, err := runtime.Logs(ctx, fs.Arg(0), *lines)
		if err != nil {
			return err
		}

		fmt.Println(output)
		return nil

	default:
		return usage()
	}
}

func usage() error {
	return errors.New(
		"usage: tyravex-docker " +
			"minecraft-install [-id ID] [-port 25565] " +
			"[-memory 2048] [-cpu 100] [-image IMAGE] | " +
			"start ID | stop ID | restart ID | status ID | " +
			"logs [-lines N] ID",
	)
}
