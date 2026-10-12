// zainium-install is the install engine; it runs as root (the GUI starts it through pkexec).
//
//	zainium-install --plan plan.json        install, JSON progress lines on stdout
//	zainium-install --plan - --text         plan on stdin, readable progress
//	zainium-install --list-disks            disks as JSON
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/engine"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
	"github.com/Zainium-Dynamics/zainium-installer/internal/progress"
)

func main() {
	// a closed GUI must not kill a half-done install: progress writes then just fail
	signal.Ignore(syscall.SIGPIPE, syscall.SIGHUP)
	planPath := flag.String("plan", "", "install plan (JSON file, or - for stdin)")
	source := flag.String("source", "", "live image tree to install (default: the running live erofs)")
	dryRun := flag.Bool("dry-run", false, "check the plan only, change nothing")
	text := flag.Bool("text", false, "readable progress instead of JSON lines")
	listDisks := flag.Bool("list-disks", false, "print the disks as JSON and exit")
	flag.Parse()

	if *listDisks {
		ds, err := disk.List()
		if err != nil {
			fatal(err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(ds)
		return
	}
	if *planPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	var in io.Reader = os.Stdin
	if *planPath != "-" {
		f, err := os.Open(*planPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in = f
	}
	var p plan.Plan
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		fatal(fmt.Errorf("reading the plan: %w", err))
	}

	var out io.Writer = os.Stdout
	if *text {
		out = &textWriter{w: os.Stdout}
	}
	r := progress.NewReporter(out)
	e := &engine.Engine{Plan: p, R: r, Source: *source, DryRun: *dryRun}
	if err := e.Run(); err != nil {
		r.Fail(err)
		os.Exit(1)
	}
	r.Done()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "zainium-install:", err)
	os.Exit(1)
}

// textWriter turns the JSON lines into readable output for a terminal.
type textWriter struct{ w io.Writer }

func (t *textWriter) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var ev progress.Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch {
		case ev.Error != "":
			fmt.Fprintf(t.w, "FAILED: %s\n", ev.Error)
		case ev.Done:
			fmt.Fprintln(t.w, "[100%] Installation complete")
		case ev.Log != "":
			fmt.Fprintf(t.w, "       %s\n", ev.Log)
		default:
			fmt.Fprintf(t.w, "[%3.0f%%] %s\n", ev.Percent, ev.Message)
		}
	}
	return len(b), nil
}
