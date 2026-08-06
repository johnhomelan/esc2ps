package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// Setup command-line flags
	inputPath := flag.String("i", "", "Path to captured Epson ESC/P2 binary file (reads from stdin if omitted)")
	outputPath := flag.String("o", "", "Path to output PostScript file (writes to stdout if omitted)")
	versionFlag := flag.Bool("v", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ESC/P2 to PostScript Converter (esc2ps)\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  esc2ps [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  esc2ps -i capture.bin -o printout.ps\n")
		fmt.Fprintf(os.Stderr, "  cat capture.bin | esc2ps > printout.ps\n")
	}

	flag.Parse()

	if *versionFlag {
		fmt.Println("esc2ps version 0.1.2")
		return
	}

	// Open input stream
	var input ioReader = os.Stdin
	if *inputPath != "" {
		file, err := os.Open(*inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	}

	// Parse input binary data
	pages, err := Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ESC/P2 input: %v\n", err)
		os.Exit(1)
	}

	if len(pages) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: No printed pages detected in input data.\n")
	}

	// Open output stream
	var output ioWriter = os.Stdout
	if *outputPath != "" {
		file, err := os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		output = file
	}

	// Generate and write PostScript file
	err = GeneratePostScript(pages, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating PostScript: %v\n", err)
		os.Exit(1)
	}
}

// Simple interface definitions to allow for stdin/stdout and file streams
type ioReader interface {
	Read(p []byte) (n int, err error)
}

type ioWriter interface {
	Write(p []byte) (n int, err error)
}
type ioCloser interface {
	Close() error
}
type ioReadCloser interface {
	ioReader
	ioCloser
}
type ioWriteCloser interface {
	ioWriter
	ioCloser
}
