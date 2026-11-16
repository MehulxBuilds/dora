package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/MehulxBuilds/dora/cli/server"
	"github.com/MehulxBuilds/dora/engine"
)

func Run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Println("Usage: dora studio [--path ./dora] [--port 7432] [--no-open] [database-file]\nExample: dora studio --path ./data\nOffline mode: stop the application before inspecting its files.")
		return nil
	}
	if args[0] != "studio" {
		return fmt.Errorf("unknown command %q (use dora --help)", args[0])
	}
	flags := flag.NewFlagSet("studio", flag.ContinueOnError)
	port := flags.Int("port", 7432, "local port (0 selects an available port)")
	directory := flags.String("path", "dora", "directory containing Dora databases (relative to the current working directory)")
	noOpen := flags.Bool("no-open", false, "do not open a browser")
	// Accept flags before or after the filename.
	var options, positional []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if a == "--port" || a == "-port" || a == "--path" || a == "-path" {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", a)
			}
			options = append(options, a, args[i+1])
			i++
			continue
		}
		if len(a) > 0 && a[0] == '-' {
			options = append(options, a)
		} else {
			positional = append(positional, a)
		}
	}
	if err := flags.Parse(options); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if len(positional) > 1 {
		return fmt.Errorf("provide at most one database file, or use --path <directory>")
	}
	var databases []database
	var err error
	if len(positional) == 1 {
		explicitPath := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "path" {
				explicitPath = true
			}
		})
		if explicitPath {
			return fmt.Errorf("use either --path <directory> or a database file")
		}
		path, absErr := filepath.Abs(positional[0])
		if absErr != nil {
			return absErr
		}
		if _, err = engine.InspectFile(path); err != nil {
			return err
		}
		databases = []database{{Name: filepath.Base(path), Path: path}}
	} else {
		databases, err = discover(*directory)
		if err != nil {
			return err
		}
	}
	sources := make(map[string]server.Source, len(databases))
	for _, database := range databases {
		path := database.Path
		sources[database.Name] = func() (*engine.Inspection, error) { return engine.InspectFile(path) }
	}
	name := databases[0].Name
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Println("Dora Studio / read-only / offline files (stop the application before using this mode)")

	for _, database := range databases {
		fmt.Println("Database:", database.Path)
	}

	return server.Serve(ctx, sources[name],
		server.Options{
			Port:      *port,
			Name:      name,
			Databases: sources,
			Mode:      "offline",
			NoBrowser: *noOpen,
			Ready: func(address string) {
				fmt.Println("Studio:", address)
				fmt.Println("Press Ctrl+C to stop.")
			},
		})
}
