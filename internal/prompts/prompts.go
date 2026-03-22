package prompts

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*.txt
var templateFS embed.FS

type Pack struct {
	templates map[string]*template.Template
	raw       map[string]string
}

func Load() (Pack, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return Pack{}, err
	}

	pack := Pack{
		templates: make(map[string]*template.Template, len(entries)),
		raw:       make(map[string]string, len(entries)),
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.ToSlash(filepath.Join("templates", entry.Name()))
		payload, err := templateFS.ReadFile(path)
		if err != nil {
			return Pack{}, err
		}

		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		source := string(payload)
		tmpl, err := template.New(name).Parse(source)
		if err != nil {
			return Pack{}, err
		}

		pack.templates[name] = tmpl
		pack.raw[name] = source
	}

	return pack, nil
}

func MustLoad() Pack {
	pack, err := Load()
	if err != nil {
		panic(err)
	}
	return pack
}

func (p Pack) Raw(name string) (string, error) {
	value, ok := p.raw[name]
	if !ok {
		return "", fmt.Errorf("prompt %q not found", name)
	}
	return value, nil
}

func (p Pack) Render(name string, data any) (string, error) {
	tmpl, ok := p.templates[name]
	if !ok {
		return "", fmt.Errorf("prompt %q not found", name)
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, data); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
