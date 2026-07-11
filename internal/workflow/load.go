package workflow

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// builtinFS holds the workflow templates shipped in the binary.
//
//go:embed templates/*.json
var builtinFS embed.FS

// templateDir is the embedded directory holding the built-in templates.
const templateDir = "templates"

// Source records where a workflow definition came from.
type Source string

const (
	// SourceBuiltin marks a workflow shipped in the binary.
	SourceBuiltin Source = "builtin"
	// SourceUser marks a workflow loaded from the user directory.
	SourceUser Source = "user"
)

// Info summarizes a workflow available to load.
type Info struct {
	// Name is the workflow name, matching its file name without the extension.
	Name string
	// Description is the one-line summary, empty when the file omits it.
	Description string
	// Source is where the definition came from.
	Source Source
}

// Loader loads workflows from an optional user directory, falling back to the
// built-in templates embedded in the binary.
type Loader struct {
	// UserDir holds user-defined workflow JSON files that override built-ins of the
	// same name. When empty, only built-ins are available.
	UserDir string
}

// Parse decodes and validates a workflow from JSON. Unknown fields are rejected so
// typos in hand-written definitions surface early.
func Parse(data []byte) (Workflow, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w Workflow
	if err := dec.Decode(&w); err != nil {
		return Workflow{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := w.Validate(); err != nil {
		return Workflow{}, err
	}
	return w, nil
}

// Load returns the workflow with the given name. A file in UserDir wins over a
// built-in template of the same name.
func (l Loader) Load(name string) (Workflow, error) {
	data, _, err := l.Raw(name)
	if err != nil {
		return Workflow{}, err
	}
	return Parse(data)
}

// Raw returns a workflow's definition bytes and their source, without parsing, so a
// caller can show or edit the JSON as written. A file in UserDir wins over a built-in
// template of the same name.
func (l Loader) Raw(name string) ([]byte, Source, error) {
	if !validName(name) {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownWorkflow, name)
	}
	if l.UserDir != "" {
		data, err := os.ReadFile(filepath.Join(l.UserDir, name+".json"))
		switch {
		case err == nil:
			return data, SourceUser, nil
		case !errors.Is(err, os.ErrNotExist):
			return nil, "", fmt.Errorf("read workflow %q: %w", name, err)
		}
	}
	data, err := builtinFS.ReadFile(path.Join(templateDir, name+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownWorkflow, name)
	}
	if err != nil {
		return nil, "", err
	}
	return data, SourceBuiltin, nil
}

// List returns every available workflow, user definitions overriding built-ins of
// the same name, sorted by name.
func (l Loader) List() ([]Info, error) {
	infos := make(map[string]Info)

	entries, err := builtinFS.ReadDir(templateDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		data, rerr := builtinFS.ReadFile(path.Join(templateDir, e.Name()))
		if rerr != nil {
			return nil, rerr
		}
		infos[name] = Info{Name: name, Description: describe(data), Source: SourceBuiltin}
	}

	if l.UserDir != "" {
		userEntries, uerr := os.ReadDir(l.UserDir)
		if uerr != nil && !errors.Is(uerr, os.ErrNotExist) {
			return nil, uerr
		}
		for _, e := range userEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".json")
			data, rerr := os.ReadFile(filepath.Join(l.UserDir, e.Name()))
			if rerr != nil {
				continue
			}
			infos[name] = Info{Name: name, Description: describe(data), Source: SourceUser}
		}
	}

	out := make([]Info, 0, len(infos))
	for _, in := range infos {
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// describe extracts a workflow's description without full validation, so listings
// tolerate a definition that parses but would fail to run.
func describe(data []byte) string {
	var meta struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &meta)
	return meta.Description
}

// validName reports whether name is a safe workflow name: non-empty and free of
// path separators or dots, so it cannot escape the user directory.
func validName(name string) bool {
	if name == "" {
		return false
	}
	return !strings.ContainsAny(name, `/\.`)
}
