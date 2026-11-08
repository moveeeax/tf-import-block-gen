package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/moveeeax/tf-import-block-gen/internal/match"
)

var outcome = match.Outcome{
	Matched: []match.Matched{
		{Address: "aws_s3_bucket.assets", Type: "aws_s3_bucket", ID: "acme-assets", RawID: "arn:aws:s3:::acme-assets", Rule: match.RuleName, Verified: true},
		{Address: "cloudflare_record.apex", Type: "cloudflare_record", ID: "abc", RawID: "abc", Rule: match.RuleName, Note: "id format for cloudflare_record is not verified -- check it before applying"},
	},
	Ambiguous: []match.Ambiguous{
		{Address: "aws_instance.web", Type: "aws_instance", PlannedName: "web", Candidates: []string{"i-0a", "i-0b"}},
	},
	New:       []match.New{{Address: "aws_dynamodb_table.sessions", Type: "aws_dynamodb_table", PlannedName: "sessions"}},
	Unmanaged: []match.Unmanaged{{Type: "aws_security_group", ID: "sg-0dead", Name: "legacy"}},
}

func TestImportBlocks(t *testing.T) {
	got := ImportBlocks(outcome)
	if n := strings.Count(got, "import {"); n != 2 {
		t.Fatalf("got %d blocks, want 2\n%s", n, got)
	}
	if !strings.Contains(got, "  to = aws_s3_bucket.assets\n  id = \"acme-assets\"\n") {
		t.Errorf("block body is wrong\n%s", got)
	}
	if !strings.Contains(got, "# id format for cloudflare_record is not verified") {
		t.Errorf("an unverified block must carry its note\n%s", got)
	}
	// Only a planned create gets a block: nothing else leaks in.
	for _, unwanted := range []string{"aws_instance.web", "aws_dynamodb_table.sessions", "sg-0dead"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%s must not appear in the blocks\n%s", unwanted, got)
		}
	}
}

func TestImportBlocksWithNoMatches(t *testing.T) {
	got := ImportBlocks(match.Outcome{})
	if strings.Contains(got, "import {") {
		t.Errorf("want no blocks, got\n%s", got)
	}
	if !strings.Contains(got, "No confident matches") {
		t.Errorf("want an explanatory comment, got\n%s", got)
	}
}

func TestReportTable(t *testing.T) {
	got := ReportTable(outcome)
	for _, want := range []string{
		"MATCHED (2)", "AMBIGUOUS (1)", "NEW (1)", "UNMANAGED (1)",
		"i-0a, i-0b", "via name", "id format unverified",
		"2 matched, 1 ambiguous, 1 new, 1 unmanaged",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q\n%s", want, got)
		}
	}
}

func TestReportTableEmptySections(t *testing.T) {
	got := ReportTable(match.Outcome{})
	if !strings.Contains(got, "MATCHED (0)") || !strings.Contains(got, "0 matched, 0 ambiguous, 0 new, 0 unmanaged") {
		t.Errorf("unexpected empty report\n%s", got)
	}
}

func TestReportJSONUsesEmptyArraysNotNull(t *testing.T) {
	got, err := ReportJSON(match.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "null") {
		t.Errorf("JSON report should use [] rather than null\n%s", got)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(got), &probe); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"matched", "ambiguous", "new", "unmanaged", "summary"} {
		if _, ok := probe[key]; !ok {
			t.Errorf("report has no %q key", key)
		}
	}
}

func TestReportJSONContent(t *testing.T) {
	raw, err := ReportJSON(outcome)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Ambiguous []struct {
			Candidates []string `json:"candidates"`
		} `json:"ambiguous"`
		Summary struct {
			Matched   int `json:"matched"`
			Ambiguous int `json:"ambiguous"`
			New       int `json:"new"`
			Unmanaged int `json:"unmanaged"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary.Matched != 2 || got.Summary.Ambiguous != 1 || got.Summary.New != 1 || got.Summary.Unmanaged != 1 {
		t.Errorf("summary = %+v", got.Summary)
	}
	if len(got.Ambiguous) != 1 || len(got.Ambiguous[0].Candidates) != 2 {
		t.Errorf("ambiguous candidates lost: %+v", got.Ambiguous)
	}
}
