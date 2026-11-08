// Package tfplan reads the JSON plan produced by `terraform show -json tfplan`
// and exposes the resources the plan wants to create.
package tfplan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Plan is the subset of the Terraform plan JSON this tool needs.
type Plan struct {
	FormatVersion   string           `json:"format_version"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
}

// ResourceChange mirrors one entry of the plan's resource_changes array.
type ResourceChange struct {
	Address       string `json:"address"`
	ModuleAddress string `json:"module_address"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	Change        Change `json:"change"`
}

// Change holds the planned action and the attributes the config asked for.
type Change struct {
	Actions []string       `json:"actions"`
	After   map[string]any `json:"after"`
}

// IsCreate reports whether the change is a plain create. A replace
// (["delete","create"]) is deliberately excluded: the object is already in
// state, so it needs no import block.
func (rc ResourceChange) IsCreate() bool {
	return len(rc.Change.Actions) == 1 && rc.Change.Actions[0] == "create"
}

// Load reads a plan from path, or from stdin when path is "-".
func Load(path string) (*Plan, error) {
	var (
		raw []byte
		err error
	)
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
		path = "<stdin>"
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return Parse(raw, path)
}

// Parse decodes plan JSON. name is used only in error messages.
func Parse(raw []byte, name string) (*Plan, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("%s: plan file is empty", name)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s: not a Terraform plan JSON document: %w", name, err)
	}
	if _, ok := probe["resource_changes"]; !ok {
		return nil, fmt.Errorf("%s: no resource_changes in plan (run `terraform show -json tfplan`)", name)
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%s: cannot decode resource_changes: %w", name, err)
	}
	for i, rc := range p.ResourceChanges {
		if rc.Address == "" || rc.Type == "" {
			return nil, fmt.Errorf("%s: resource_changes[%d] has no address or type", name, i)
		}
	}
	return &p, nil
}

// Creates returns every managed resource the plan wants to create.
func (p *Plan) Creates() []ResourceChange {
	var out []ResourceChange
	for _, rc := range p.ResourceChanges {
		if rc.Mode != "" && rc.Mode != "managed" {
			continue
		}
		if rc.IsCreate() {
			out = append(out, rc)
		}
	}
	return out
}
