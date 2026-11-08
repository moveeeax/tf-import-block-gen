// Package match joins the resources a plan wants to create to the resources
// an inventory says already exist.
package match

import (
	"sort"
	"strings"

	"github.com/moveeeax/tf-import-block-gen/internal/idfmt"
	"github.com/moveeeax/tf-import-block-gen/internal/inventory"
	"github.com/moveeeax/tf-import-block-gen/internal/tfplan"
)

// Rule names how a planned resource found its live counterpart.
type Rule string

const (
	RuleOverride Rule = "override"
	RuleName     Rule = "name"
	RuleAddress  Rule = "address"
)

// Matched is a planned create with a confident live id.
type Matched struct {
	Address     string `json:"address"`
	Type        string `json:"type"`
	PlannedName string `json:"planned_name,omitempty"`
	ID          string `json:"id"`
	RawID       string `json:"raw_id"`
	Rule        Rule   `json:"rule"`
	Verified    bool   `json:"verified"`
	Note        string `json:"note,omitempty"`
}

// Ambiguous is a planned create with more than one live candidate. It is never
// guessed: emitting the wrong id puts the wrong object in state.
type Ambiguous struct {
	Address     string   `json:"address"`
	Type        string   `json:"type"`
	PlannedName string   `json:"planned_name,omitempty"`
	Candidates  []string `json:"candidates"`
}

// New is a planned create with no live counterpart -- it really is new.
type New struct {
	Address     string `json:"address"`
	Type        string `json:"type"`
	PlannedName string `json:"planned_name,omitempty"`
}

// Unmanaged is a live resource no plan wants.
type Unmanaged struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Outcome is the full result of a run.
type Outcome struct {
	Matched   []Matched   `json:"matched"`
	Ambiguous []Ambiguous `json:"ambiguous"`
	New       []New       `json:"new"`
	Unmanaged []Unmanaged `json:"unmanaged"`
}

// nameAttrs are the attribute keys that commonly hold a resource's real name,
// tried in order before falling back to the Name tag.
var nameAttrs = []string{
	"name", "bucket", "queue_name", "repository_name", "group_name",
	"function_name", "cluster_name", "identifier", "domain_name", "table_name",
}

// PlannedName is the name the config asked for: a name-ish attribute if the
// resource has one, otherwise its Name tag.
func PlannedName(rc tfplan.ResourceChange) string {
	for _, k := range nameAttrs {
		if s, ok := rc.Change.After[k].(string); ok && s != "" {
			return s
		}
	}
	if tags, ok := rc.Change.After["tags"].(map[string]any); ok {
		for k, v := range tags {
			if strings.EqualFold(k, "name") {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// normalize makes dashes and underscores interchangeable so a
// `web_server` address matches a `web-server` tag.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

// Run matches creates against inv. overrides maps a plan address to an id the
// caller supplied by hand; an override wins over every heuristic.
func Run(creates []tfplan.ResourceChange, inv []inventory.Entry, overrides map[string]string) Outcome {
	var out Outcome
	used := make(map[int]bool, len(inv))

	for _, rc := range creates {
		planned := PlannedName(rc)

		if raw, ok := overrides[rc.Address]; ok {
			out.Matched = append(out.Matched, build(rc, planned, raw, inventory.Entry{Type: rc.Type, ID: raw}, RuleOverride))
			for i, e := range inv {
				if e.Type == rc.Type && e.ID == raw {
					used[i] = true
				}
			}
			continue
		}

		idx, rule := candidates(rc, planned, inv)
		switch len(idx) {
		case 1:
			e := inv[idx[0]]
			used[idx[0]] = true
			out.Matched = append(out.Matched, build(rc, planned, e.ID, e, rule))
		case 0:
			out.New = append(out.New, New{Address: rc.Address, Type: rc.Type, PlannedName: planned})
		default:
			cands := make([]string, 0, len(idx))
			for _, i := range idx {
				used[i] = true
				cands = append(cands, inv[i].ID)
			}
			sort.Strings(cands)
			out.Ambiguous = append(out.Ambiguous, Ambiguous{
				Address: rc.Address, Type: rc.Type, PlannedName: planned, Candidates: cands,
			})
		}
	}

	for i, e := range inv {
		if !used[i] {
			out.Unmanaged = append(out.Unmanaged, Unmanaged{Type: e.Type, ID: e.ID, Name: e.Label()})
		}
	}
	return out
}

// candidates applies the two heuristics in order: an exact name match first,
// then the normalised address name. The first stage that produces any hit wins,
// so a weaker rule never dilutes a strong one into an ambiguity.
func candidates(rc tfplan.ResourceChange, planned string, inv []inventory.Entry) ([]int, Rule) {
	var byName []int
	if planned != "" {
		for i, e := range inv {
			if e.Type != rc.Type {
				continue
			}
			if strings.EqualFold(e.Name, planned) || strings.EqualFold(e.NameTag(), planned) {
				byName = append(byName, i)
			}
		}
	}
	if len(byName) > 0 {
		return byName, RuleName
	}

	var byAddr []int
	want := normalize(rc.Name)
	if want == "" {
		return nil, RuleAddress
	}
	for i, e := range inv {
		if e.Type != rc.Type {
			continue
		}
		if normalize(e.Name) == want || normalize(e.NameTag()) == want {
			byAddr = append(byAddr, i)
		}
	}
	return byAddr, RuleAddress
}

func build(rc tfplan.ResourceChange, planned, rawID string, e inventory.Entry, rule Rule) Matched {
	res := idfmt.Format(idfmt.Input{
		Type:      rc.Type,
		ID:        rawID,
		Name:      e.Name,
		Region:    e.Region,
		AccountID: e.AccountID,
		Tags:      e.Tags,
		PlanAttrs: rc.Change.After,
	})
	return Matched{
		Address:     rc.Address,
		Type:        rc.Type,
		PlannedName: planned,
		ID:          res.ID,
		RawID:       rawID,
		Rule:        rule,
		Verified:    res.Verified,
		Note:        res.Note,
	}
}
