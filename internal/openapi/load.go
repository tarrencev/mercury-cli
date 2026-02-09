package openapi

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tarrence/mercury-cli/specs"
)

func LoadEmbeddedSpecs() ([]*SpecDoc, error) {
	entries, err := fs.Glob(specs.FS, "*.json")
	if err != nil {
		return nil, fmt.Errorf("list embedded specs: %w", err)
	}
	sort.Strings(entries)

	var out []*SpecDoc
	for _, filename := range entries {
		b, err := fs.ReadFile(specs.FS, filename)
		if err != nil {
			return nil, fmt.Errorf("read embedded spec %q: %w", filename, err)
		}
		var spec Spec
		if err := json.Unmarshal(b, &spec); err != nil {
			return nil, fmt.Errorf("parse embedded spec %q: %w", filename, err)
		}
		name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		out = append(out, &SpecDoc{
			Name:     name,
			Filename: filepath.Base(filename),
			Spec:     &spec,
		})
	}
	return out, nil
}
