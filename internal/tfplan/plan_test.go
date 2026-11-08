package tfplan

import (
	"strings"
	"testing"
)

const plan = `{
  "format_version": "1.2",
  "resource_changes": [
    {"address":"aws_s3_bucket.a","mode":"managed","type":"aws_s3_bucket","name":"a",
     "change":{"actions":["create"],"after":{"bucket":"acme"}}},
    {"address":"aws_instance.b","mode":"managed","type":"aws_instance","name":"b",
     "change":{"actions":["delete","create"],"after":{}}},
    {"address":"aws_vpc.c","mode":"managed","type":"aws_vpc","name":"c",
     "change":{"actions":["no-op"],"after":{}}},
    {"address":"data.aws_ami.d","mode":"data","type":"aws_ami","name":"d",
     "change":{"actions":["read"],"after":{}}},
    {"address":"aws_subnet.e","mode":"managed","type":"aws_subnet","name":"e",
     "change":{"actions":["update"],"after":{}}}
  ]
}`

func TestCreatesKeepsOnlyPlainCreates(t *testing.T) {
	p, err := Parse([]byte(plan), "tfplan.json")
	if err != nil {
		t.Fatal(err)
	}
	got := p.Creates()
	if len(got) != 1 {
		t.Fatalf("got %d creates, want 1: %+v", len(got), got)
	}
	if got[0].Address != "aws_s3_bucket.a" {
		t.Errorf("address = %q", got[0].Address)
	}
}

func TestParseErrorsNameTheFile(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"broken json":         `{"resource_changes": [`,
		"no resource_changes": `{"format_version":"1.0"}`,
		"entry with no type":  `{"resource_changes":[{"address":"x"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(body), "tfplan.json")
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "tfplan.json") {
				t.Errorf("error should name the file: %v", err)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Errorf("want a single-line error: %v", err)
			}
		})
	}
}

func TestIsCreate(t *testing.T) {
	tests := []struct {
		actions []string
		want    bool
	}{
		{[]string{"create"}, true},
		{[]string{"delete", "create"}, false},
		{[]string{"create", "delete"}, false},
		{[]string{"update"}, false},
		{nil, false},
	}
	for _, tc := range tests {
		rc := ResourceChange{Change: Change{Actions: tc.actions}}
		if got := rc.IsCreate(); got != tc.want {
			t.Errorf("IsCreate(%v) = %v, want %v", tc.actions, got, tc.want)
		}
	}
}
