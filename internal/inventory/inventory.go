// Package inventory reads a list of resources that already exist in the
// account. Nothing here talks to a cloud API: the inventory is a file, so the
// tool runs in CI with no credentials.
package inventory

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Entry is one live resource.
type Entry struct {
	Type      string            `json:"type"`
	ID        string            `json:"id"`
	Name      string            `json:"name,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Region    string            `json:"region,omitempty"`
	AccountID string            `json:"account_id,omitempty"`
}

// NameTag returns the value of the Name tag, case-insensitively.
func (e Entry) NameTag() string {
	for k, v := range e.Tags {
		if strings.EqualFold(k, "name") {
			return v
		}
	}
	return ""
}

// Label is a short human identifier for reports.
func (e Entry) Label() string {
	if n := e.Name; n != "" {
		return n
	}
	if n := e.NameTag(); n != "" {
		return n
	}
	return e.ID
}

type jsonWrapper struct {
	Resources []Entry `json:"resources"`
}

// Load reads an inventory from path ("-" means stdin). A .csv extension
// selects the CSV reader, anything else is parsed as JSON.
func Load(path string) ([]Entry, error) {
	var (
		raw []byte
		err error
	)
	name := path
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
		name = "<stdin>"
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".csv") {
		return ParseCSV(raw, name)
	}
	return ParseJSON(raw, name)
}

// ParseJSON accepts either a top-level array of entries or an object with a
// "resources" array.
func ParseJSON(raw []byte, name string) ([]Entry, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("%s: inventory file is empty", name)
	}
	var entries []Entry
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("%s: cannot decode inventory array: %w", name, err)
		}
	} else {
		var w jsonWrapper
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, fmt.Errorf("%s: cannot decode inventory object: %w", name, err)
		}
		if w.Resources == nil {
			return nil, fmt.Errorf("%s: inventory object has no \"resources\" array", name)
		}
		entries = w.Resources
	}
	return validate(entries, name)
}

// ParseCSV reads a header row and then one entry per line. Recognised columns
// are type, id, name, tags, region and account_id; tags use "k=v;k=v".
func ParseCSV(raw []byte, name string) ([]Entry, error) {
	r := csv.NewReader(strings.NewReader(string(raw)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: cannot decode CSV: %w", name, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: inventory file is empty", name)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	if _, ok := col["type"]; !ok {
		return nil, fmt.Errorf("%s: CSV header has no \"type\" column", name)
	}
	if _, ok := col["id"]; !ok {
		return nil, fmt.Errorf("%s: CSV header has no \"id\" column", name)
	}
	at := func(row []string, key string) string {
		i, ok := col[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var entries []Entry
	for _, row := range rows[1:] {
		if len(row) == 0 || strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		e := Entry{
			Type:      at(row, "type"),
			ID:        at(row, "id"),
			Name:      at(row, "name"),
			Region:    at(row, "region"),
			AccountID: at(row, "account_id"),
		}
		if tags := at(row, "tags"); tags != "" {
			e.Tags = map[string]string{}
			for _, pair := range strings.Split(tags, ";") {
				k, v, ok := strings.Cut(pair, "=")
				if !ok {
					continue
				}
				e.Tags[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		entries = append(entries, e)
	}
	return validate(entries, name)
}

func validate(entries []Entry, name string) ([]Entry, error) {
	for i, e := range entries {
		if e.Type == "" || e.ID == "" {
			return nil, fmt.Errorf("%s: inventory entry %d needs both a type and an id", name, i+1)
		}
	}
	return entries, nil
}
