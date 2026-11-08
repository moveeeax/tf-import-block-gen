package inventory

import (
	"strings"
	"testing"
)

func TestParseJSONArray(t *testing.T) {
	got, err := ParseJSON([]byte(`[{"type":"aws_vpc","id":"vpc-1","name":"core"}]`), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "vpc-1" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseJSONWrapper(t *testing.T) {
	got, err := ParseJSON([]byte(`{"resources":[{"type":"aws_vpc","id":"vpc-1"}]}`), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseJSONErrors(t *testing.T) {
	cases := map[string]string{
		"empty":            ``,
		"broken":           `[{"type":`,
		"no resources key": `{"items":[]}`,
		"entry with no id": `[{"type":"aws_vpc"}]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseJSON([]byte(body), "live.json"); err == nil {
				t.Fatal("want an error")
			} else if !strings.Contains(err.Error(), "live.json") {
				t.Errorf("error should name the file: %v", err)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	body := "type,id,name,tags,region\n" +
		"aws_instance,i-0abc,,Name=web;env=prod,eu-central-1\n" +
		"\n" +
		"aws_vpc,vpc-1,core,,eu-central-1\n"
	got, err := ParseCSV([]byte(body), "live.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got[0].NameTag() != "web" {
		t.Errorf("Name tag = %q", got[0].NameTag())
	}
	if got[0].Tags["env"] != "prod" {
		t.Errorf("tags = %v", got[0].Tags)
	}
	if got[0].Label() != "web" {
		t.Errorf("label = %q, want the Name tag", got[0].Label())
	}
	if got[1].Label() != "core" {
		t.Errorf("label = %q, want the name column", got[1].Label())
	}
}

func TestParseCSVNeedsTypeAndID(t *testing.T) {
	if _, err := ParseCSV([]byte("name,id\nweb,i-0abc\n"), "live.csv"); err == nil {
		t.Fatal("want an error about the missing type column")
	}
}

func TestLabelFallsBackToID(t *testing.T) {
	e := Entry{Type: "aws_vpc", ID: "vpc-1"}
	if e.Label() != "vpc-1" {
		t.Errorf("label = %q", e.Label())
	}
}
