package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// Acceptance: a plan whose creates all exist emits one block per create, with
// the plan address verbatim and the id formatted per type, and exits 0.
func TestAllCreatesMatch(t *testing.T) {
	code, stdout, stderr := run(t,
		"--plan", "testdata/plan_all_match.json",
		"--inventory", "testdata/live_all_match.json",
	)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\nstderr:\n%s", code, ExitOK, stderr)
	}
	if n := strings.Count(stdout, "import {"); n != 3 {
		t.Fatalf("got %d import blocks, want 3\n%s", n, stdout)
	}
	for _, want := range []string{
		"to = aws_s3_bucket.assets",
		`id = "acme-assets-prod"`,
		"to = aws_iam_role.app",
		`id = "acme-app-task"`,
		"to = aws_instance.web_server",
		`id = "i-0abc123def4567890"`,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q\n%s", want, stdout)
		}
	}
	// A replace (delete+create) is already in state and must not be imported.
	if strings.Contains(stdout, "aws_security_group.db") {
		t.Error("a replaced resource must not get an import block")
	}
	// The queue nobody plans is reported as unmanaged.
	if !strings.Contains(stderr, "acme-jobs") {
		t.Errorf("report missing the unmanaged queue\n%s", stderr)
	}
	if !strings.Contains(stderr, "3 matched, 0 ambiguous, 0 new, 1 unmanaged") {
		t.Errorf("unexpected summary\n%s", stderr)
	}
}

// The CSV reader must produce the same matches as the JSON one.
func TestCSVInventoryMatchesJSON(t *testing.T) {
	_, jsonOut, _ := run(t, "--plan", "testdata/plan_all_match.json", "--inventory", "testdata/live_all_match.json")
	_, csvOut, _ := run(t, "--plan", "testdata/plan_all_match.json", "--inventory", "testdata/live_all_match.csv")
	if jsonOut != csvOut {
		t.Errorf("CSV and JSON inventories disagree\nJSON:\n%s\nCSV:\n%s", jsonOut, csvOut)
	}
}

// Acceptance: two candidates means no block, both ids listed, exit 1.
func TestAmbiguousIsNeverGuessed(t *testing.T) {
	code, stdout, stderr := run(t,
		"--plan", "testdata/plan_ambiguous.json",
		"--inventory", "testdata/live_ambiguous.json",
	)
	if code != ExitFinding {
		t.Fatalf("exit = %d, want %d", code, ExitFinding)
	}
	if strings.Contains(stdout, "import {") {
		t.Errorf("an ambiguous match must not emit a block\n%s", stdout)
	}
	for _, id := range []string{"i-0aaaaaaaaaaaaaaa1", "i-0bbbbbbbbbbbbbbb2"} {
		if !strings.Contains(stderr, id) {
			t.Errorf("report missing candidate %s\n%s", id, stderr)
		}
	}
}

// An override resolves what the heuristics refuse to guess.
func TestMapOverrideResolvesAmbiguity(t *testing.T) {
	code, stdout, _ := run(t,
		"--plan", "testdata/plan_ambiguous.json",
		"--inventory", "testdata/live_ambiguous.json",
		"--map", "aws_instance.web=i-0bbbbbbbbbbbbbbb2",
	)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, `id = "i-0bbbbbbbbbbbbbbb2"`) {
		t.Errorf("override id missing\n%s", stdout)
	}
}

func TestMalformedOverrideIsRejected(t *testing.T) {
	code, _, stderr := run(t,
		"--plan", "testdata/plan_ambiguous.json",
		"--inventory", "testdata/live_ambiguous.json",
		"--map", "aws_instance.web",
	)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--map") {
		t.Errorf("error should name --map\n%s", stderr)
	}
}

// Acceptance: a create with no candidate is "new", not an error -- unless --strict.
func TestGenuinelyNewResource(t *testing.T) {
	code, stdout, stderr := run(t, "--plan", "testdata/plan_new.json", "--inventory", "testdata/live_new.json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stderr, "aws_dynamodb_table.sessions") {
		t.Errorf("report missing the new resource\n%s", stderr)
	}
	if strings.Contains(stdout, "aws_dynamodb_table.sessions") {
		t.Errorf("a new resource must not get an import block\n%s", stdout)
	}

	code, _, _ = run(t, "--plan", "testdata/plan_new.json", "--inventory", "testdata/live_new.json", "--strict")
	if code != ExitFinding {
		t.Fatalf("--strict exit = %d, want %d", code, ExitFinding)
	}
}

// Acceptance: an unknown type still emits a usable block, flagged by a comment.
func TestUnknownTypeStillEmitsAFlaggedBlock(t *testing.T) {
	code, stdout, _ := run(t,
		"--plan", "testdata/plan_unknown_type.json",
		"--inventory", "testdata/live_unknown_type.json",
	)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, `id = "a1b2c3d4e5f60718293a4b5c6d7e8f90"`) {
		t.Errorf("raw id missing\n%s", stdout)
	}
	if !strings.Contains(stdout, "# id format for cloudflare_record is not verified") {
		t.Errorf("unverified comment missing\n%s", stdout)
	}
}

// Acceptance: a malformed plan exits 2 with a one-line message naming the file.
func TestMalformedPlanExitsTwo(t *testing.T) {
	code, _, stderr := run(t,
		"--plan", "testdata/plan_malformed.json",
		"--inventory", "testdata/live_new.json",
	)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "plan_malformed.json") {
		t.Errorf("error should name the file\n%s", stderr)
	}
	if lines := strings.Count(strings.TrimSpace(stderr), "\n"); lines != 0 {
		t.Errorf("want a one-line error, got:\n%s", stderr)
	}
	if strings.Contains(stderr, "goroutine") {
		t.Error("panicked instead of reporting an error")
	}
}

func TestPlanWithoutResourceChangesIsRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	if err := os.WriteFile(p, []byte(`{"format_version":"1.0","values":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "--plan", p, "--inventory", "testdata/live_new.json")
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "no resource_changes") {
		t.Errorf("unhelpful error:\n%s", stderr)
	}
}

func TestOutFileAndTypeFilters(t *testing.T) {
	out := filepath.Join(t.TempDir(), "imports.tf")
	code, stdout, _ := run(t,
		"--plan", "testdata/plan_all_match.json",
		"--inventory", "testdata/live_all_match.json",
		"--only", "aws_s3_bucket",
		"--out", out,
	)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if stdout != "" {
		t.Errorf("--out must keep stdout clean, got %q", stdout)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if n := strings.Count(body, "import {"); n != 1 {
		t.Fatalf("--only should leave 1 block, got %d\n%s", n, body)
	}
	if !strings.Contains(body, "aws_s3_bucket.assets") {
		t.Errorf("wrong block survived --only\n%s", body)
	}

	code, _, stderr := run(t,
		"--plan", "testdata/plan_all_match.json",
		"--inventory", "testdata/live_all_match.json",
		"--skip", "aws_s3_bucket,aws_iam_role",
	)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stderr, "1 matched") {
		t.Errorf("--skip summary unexpected\n%s", stderr)
	}
}

func TestJSONReportIsValid(t *testing.T) {
	_, _, stderr := run(t,
		"--plan", "testdata/plan_all_match.json",
		"--inventory", "testdata/live_all_match.json",
		"--format", "json",
	)
	var got struct {
		Matched []struct {
			Address  string `json:"address"`
			ID       string `json:"id"`
			Rule     string `json:"rule"`
			Verified bool   `json:"verified"`
		} `json:"matched"`
		Summary struct {
			Matched   int `json:"matched"`
			Unmanaged int `json:"unmanaged"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("report is not valid JSON: %v\n%s", err, stderr)
	}
	if got.Summary.Matched != 3 || got.Summary.Unmanaged != 1 {
		t.Errorf("summary = %+v", got.Summary)
	}
	for _, m := range got.Matched {
		if !m.Verified {
			t.Errorf("%s should have a verified id format", m.Address)
		}
	}
}

func TestMissingFlagsAndBadFormat(t *testing.T) {
	if code, _, _ := run(t, "--plan", "testdata/plan_new.json"); code != ExitError {
		t.Errorf("missing --inventory exit = %d, want %d", code, ExitError)
	}
	if code, _, _ := run(t,
		"--plan", "testdata/plan_new.json",
		"--inventory", "testdata/live_new.json",
		"--format", "yaml",
	); code != ExitError {
		t.Errorf("bad --format exit = %d, want %d", code, ExitError)
	}
}

func TestVersionAndListTypes(t *testing.T) {
	code, stdout, _ := run(t, "--version")
	if code != ExitOK || !strings.Contains(stdout, "tf-import-block-gen") {
		t.Errorf("--version: exit %d, stdout %q", code, stdout)
	}
	code, stdout, _ = run(t, "--list-types")
	if code != ExitOK || !strings.Contains(stdout, "aws_route53_record") {
		t.Errorf("--list-types: exit %d, stdout %q", code, stdout)
	}
}

// The examples in the README must keep working.
func TestExamplesDirectory(t *testing.T) {
	code, stdout, stderr := run(t,
		"--plan", "../../examples/tfplan.json",
		"--inventory", "../../examples/live.json",
	)
	if code != ExitFinding {
		t.Fatalf("example exit = %d, want %d (the example has an ambiguity)\n%s", code, ExitFinding, stderr)
	}
	if n := strings.Count(stdout, "import {"); n < 4 {
		t.Errorf("example produced %d blocks, want at least 4\n%s", n, stdout)
	}
}
