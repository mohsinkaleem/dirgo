package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	profileFlag := flag.Bool("profile", false, "enable CPU profiling (writes cpu.prof)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("dirgo %s\n", version)
		return
	}

	if *profileFlag {
		f, err := os.Create("cpu.prof")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not create CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	// Determine target path from positional args or default to current directory.
	path := "."
	args := flag.Args()
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Verify path exists and is a directory.
	info, err := os.Stat(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", absPath)
		os.Exit(1)
	}

	model := NewModel(absPath)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
