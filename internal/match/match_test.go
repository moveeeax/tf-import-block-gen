package match

import (
	"testing"

	"github.com/moveeeax/tf-import-block-gen/internal/inventory"
	"github.com/moveeeax/tf-import-block-gen/internal/tfplan"
)

func create(addr, typ, name string, after map[string]any) tfplan.ResourceChange {
	return tfplan.ResourceChange{
		Address: addr, Mode: "managed", Type: typ, Name: name,
		Change: tfplan.Change{Actions: []string{"create"}, After: after},
	}
}

func TestPlannedName(t *testing.T) {
	tests := []struct {
		name string
		rc   tfplan.ResourceChange
		want string
	}{
		{
			name: "bucket attribute",
			rc:   create("aws_s3_bucket.a", "aws_s3_bucket", "a", map[string]any{"bucket": "acme-assets"}),
			want: "acme-assets",
		},
		{
			name: "name attribute wins over the tag",
			rc:   create("aws_iam_role.a", "aws_iam_role", "a", map[string]any{"name": "task", "tags": map[string]any{"Name": "other"}}),
			want: "task",
		},
		{
			name: "Name tag when the resource has no name attribute",
			rc:   create("aws_instance.web", "aws_instance", "web", map[string]any{"tags": map[string]any{"Name": "acme-web"}}),
			want: "acme-web",
		},
		{
			name: "nothing to go on",
			rc:   create("aws_instance.web", "aws_instance", "web", map[string]any{"instance_type": "t3.small"}),
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlannedName(tc.rc); got != tc.want {
				t.Errorf("PlannedName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExactNameMatch(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_s3_bucket.assets", "aws_s3_bucket", "assets", map[string]any{"bucket": "acme-assets"}),
	}
	inv := []inventory.Entry{{Type: "aws_s3_bucket", ID: "acme-assets", Name: "acme-assets"}}

	out := Run(creates, inv, nil)
	if len(out.Matched) != 1 {
		t.Fatalf("matched = %+v", out)
	}
	if out.Matched[0].Rule != RuleName {
		t.Errorf("rule = %q, want %q", out.Matched[0].Rule, RuleName)
	}
	if len(out.Unmanaged) != 0 {
		t.Errorf("a matched entry must not also be unmanaged: %+v", out.Unmanaged)
	}
}

// A `web_server` address should find a `web-server` tag.
func TestAddressMatchNormalisesSeparators(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_instance.web_server", "aws_instance", "web_server", map[string]any{"instance_type": "t3.small"}),
	}
	inv := []inventory.Entry{{Type: "aws_instance", ID: "i-0abc", Tags: map[string]string{"Name": "WEB-SERVER"}}}

	out := Run(creates, inv, nil)
	if len(out.Matched) != 1 || out.Matched[0].Rule != RuleAddress {
		t.Fatalf("want one address-rule match, got %+v", out)
	}
}

func TestTypeMustAgree(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_s3_bucket.assets", "aws_s3_bucket", "assets", map[string]any{"bucket": "acme"}),
	}
	inv := []inventory.Entry{{Type: "aws_sqs_queue", ID: "acme", Name: "acme"}}

	out := Run(creates, inv, nil)
	if len(out.New) != 1 || len(out.Matched) != 0 {
		t.Fatalf("a same-name different-type entry must not match: %+v", out)
	}
	if len(out.Unmanaged) != 1 {
		t.Errorf("the queue should be reported as unmanaged: %+v", out.Unmanaged)
	}
}

func TestAmbiguousKeepsEveryCandidateSorted(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_instance.web", "aws_instance", "web", map[string]any{"tags": map[string]any{"Name": "web"}}),
	}
	inv := []inventory.Entry{
		{Type: "aws_instance", ID: "i-0b", Tags: map[string]string{"Name": "web"}},
		{Type: "aws_instance", ID: "i-0a", Tags: map[string]string{"Name": "web"}},
	}

	out := Run(creates, inv, nil)
	if len(out.Ambiguous) != 1 {
		t.Fatalf("want an ambiguity, got %+v", out)
	}
	got := out.Ambiguous[0].Candidates
	if len(got) != 2 || got[0] != "i-0a" || got[1] != "i-0b" {
		t.Errorf("candidates = %v, want sorted [i-0a i-0b]", got)
	}
	if len(out.Matched) != 0 {
		t.Error("an ambiguity must never produce a match")
	}
}

// A strong name hit must not be diluted into an ambiguity by a weaker
// address-level hit on another entry.
func TestNameRuleShadowsAddressRule(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_instance.web", "aws_instance", "web", map[string]any{"tags": map[string]any{"Name": "acme-web"}}),
	}
	inv := []inventory.Entry{
		{Type: "aws_instance", ID: "i-0exact", Tags: map[string]string{"Name": "acme-web"}},
		{Type: "aws_instance", ID: "i-0loose", Tags: map[string]string{"Name": "web"}},
	}

	out := Run(creates, inv, nil)
	if len(out.Matched) != 1 || out.Matched[0].ID != "i-0exact" {
		t.Fatalf("want the exact-name entry, got %+v", out)
	}
}

func TestOverrideBeatsEverything(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_instance.web", "aws_instance", "web", map[string]any{"tags": map[string]any{"Name": "web"}}),
	}
	inv := []inventory.Entry{
		{Type: "aws_instance", ID: "i-0a", Tags: map[string]string{"Name": "web"}},
		{Type: "aws_instance", ID: "i-0b", Tags: map[string]string{"Name": "web"}},
	}

	out := Run(creates, inv, map[string]string{"aws_instance.web": "i-0b"})
	if len(out.Ambiguous) != 0 {
		t.Fatalf("the override should have settled it: %+v", out.Ambiguous)
	}
	if len(out.Matched) != 1 || out.Matched[0].ID != "i-0b" || out.Matched[0].Rule != RuleOverride {
		t.Fatalf("matched = %+v", out.Matched)
	}
	if len(out.Unmanaged) != 1 || out.Unmanaged[0].ID != "i-0a" {
		t.Errorf("unmanaged = %+v", out.Unmanaged)
	}
}

func TestUnverifiedIDIsCarriedThrough(t *testing.T) {
	creates := []tfplan.ResourceChange{
		create("aws_sqs_queue.jobs", "aws_sqs_queue", "jobs", map[string]any{"name": "jobs"}),
	}
	inv := []inventory.Entry{{Type: "aws_sqs_queue", ID: "jobs", Name: "jobs"}}

	out := Run(creates, inv, nil)
	if len(out.Matched) != 1 {
		t.Fatalf("matched = %+v", out)
	}
	m := out.Matched[0]
	if m.Verified {
		t.Error("a bare queue name cannot be a verified import id")
	}
	if m.Note == "" {
		t.Error("want a note explaining why")
	}
}

func TestNoCreatesIsAnEmptyOutcome(t *testing.T) {
	out := Run(nil, nil, nil)
	if len(out.Matched)+len(out.Ambiguous)+len(out.New)+len(out.Unmanaged) != 0 {
		t.Fatalf("want an empty outcome, got %+v", out)
	}
}
