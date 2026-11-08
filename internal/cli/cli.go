// Package cli wires the flags, the readers and the renderers together. Run is
// kept separate from main so the whole program is testable in-process.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/moveeeax/tf-import-block-gen/internal/idfmt"
	"github.com/moveeeax/tf-import-block-gen/internal/inventory"
	"github.com/moveeeax/tf-import-block-gen/internal/match"
	"github.com/moveeeax/tf-import-block-gen/internal/render"
	"github.com/moveeeax/tf-import-block-gen/internal/tfplan"
)

// Version is the build version, overridable with -ldflags.
var Version = "0.1.0"

// Exit codes.
const (
	ExitOK      = 0
	ExitFinding = 1
	ExitError   = 2
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// Run executes the tool. Import blocks go to stdout (or --out); the report
// always goes to stderr, so `tf-import-block-gen ... > imports.tf` is safe.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tf-import-block-gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		planPath  = fs.String("plan", "", "path to `terraform show -json tfplan` output (- for stdin)")
		invPath   = fs.String("inventory", "", "path to the live-resource inventory, .json or .csv (- for stdin)")
		outPath   = fs.String("out", "", "write import blocks to this file instead of stdout")
		format    = fs.String("format", "table", "report format: table or json")
		only      = fs.String("only", "", "comma-separated resource types to consider")
		skip      = fs.String("skip", "", "comma-separated resource types to ignore")
		strict    = fs.Bool("strict", false, "exit 1 when any planned create stays unmatched")
		showVer   = fs.Bool("version", false, "print the version and exit")
		showTypes = fs.Bool("list-types", false, "print the resource types with a known id format")
		maps      stringList
	)
	fs.Var(&maps, "map", "force a match: <plan.address>=<id> (repeatable)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "tf-import-block-gen generates Terraform 1.5 import blocks from a plan plus an inventory of live resources.\n\nUsage:\n  tf-import-block-gen --plan tfplan.json --inventory live.json [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	if *showVer {
		fmt.Fprintf(stdout, "tf-import-block-gen %s\n", Version)
		return ExitOK
	}
	if *showTypes {
		types := idfmt.Supported()
		sort.Strings(types)
		for _, t := range types {
			fmt.Fprintln(stdout, t)
		}
		return ExitOK
	}
	if *planPath == "" || *invPath == "" {
		fmt.Fprintln(stderr, "error: both --plan and --inventory are required")
		fs.Usage()
		return ExitError
	}
	if *format != "table" && *format != "json" {
		fmt.Fprintf(stderr, "error: --format must be table or json, got %q\n", *format)
		return ExitError
	}
	if *planPath == "-" && *invPath == "-" {
		fmt.Fprintln(stderr, "error: only one of --plan and --inventory can read stdin")
		return ExitError
	}

	overrides, err := parseOverrides(maps)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitError
	}

	plan, err := tfplan.Load(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitError
	}
	inv, err := inventory.Load(*invPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitError
	}

	keep := typeFilter(*only, *skip)
	var creates []tfplan.ResourceChange
	for _, rc := range plan.Creates() {
		if keep(rc.Type) {
			creates = append(creates, rc)
		}
	}
	var live []inventory.Entry
	for _, e := range inv {
		if keep(e.Type) {
			live = append(live, e)
		}
	}

	outcome := match.Run(creates, live, overrides)

	blocks := render.ImportBlocks(outcome)
	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(blocks), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: %s: %v\n", *outPath, err)
			return ExitError
		}
		fmt.Fprintf(stderr, "wrote %d import block(s) to %s\n", len(outcome.Matched), *outPath)
	} else {
		fmt.Fprint(stdout, blocks)
	}

	if *format == "json" {
		report, err := render.ReportJSON(outcome)
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot encode report: %v\n", err)
			return ExitError
		}
		fmt.Fprint(stderr, report)
	} else {
		fmt.Fprint(stderr, render.ReportTable(outcome))
	}

	if len(outcome.Ambiguous) > 0 {
		return ExitFinding
	}
	if *strict && len(outcome.New) > 0 {
		return ExitFinding
	}
	return ExitOK
}

func parseOverrides(maps []string) (map[string]string, error) {
	out := map[string]string{}
	for _, m := range maps {
		addr, id, ok := strings.Cut(m, "=")
		addr, id = strings.TrimSpace(addr), strings.TrimSpace(id)
		if !ok || addr == "" || id == "" {
			return nil, fmt.Errorf("--map %q is not <plan.address>=<id>", m)
		}
		out[addr] = id
	}
	return out, nil
}

func typeFilter(only, skip string) func(string) bool {
	set := func(csv string) map[string]bool {
		m := map[string]bool{}
		for _, t := range strings.Split(csv, ",") {
			if t = strings.TrimSpace(t); t != "" {
				m[t] = true
			}
		}
		return m
	}
	onlySet, skipSet := set(only), set(skip)
	return func(t string) bool {
		if len(onlySet) > 0 && !onlySet[t] {
			return false
		}
		return !skipSet[t]
	}
}
