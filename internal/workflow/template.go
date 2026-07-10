package workflow

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

type TemplateData struct {
	Vars      map[string]any    `json:"vars"`
	Stages    map[string]any    `json:"stages"`
	Revisions map[string]string `json:"revisions"`
	Run       map[string]any    `json:"run"`
}

var safeTemplateFuncs = template.FuncMap{
	"lower": strings.ToLower, "upper": strings.ToUpper, "trim": strings.TrimSpace,
	"quote": func(s string) string { return fmt.Sprintf("%q", s) },
	"join":  strings.Join, "replace": strings.ReplaceAll,
}

func Render(name, text string, data TemplateData) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Funcs(safeTemplateFuncs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	var out bytes.Buffer
	payload := map[string]any{"vars": data.Vars, "stages": data.Stages, "revisions": data.Revisions, "run": data.Run}
	if err := t.Execute(&out, payload); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return out.String(), nil
}

func renderList(name string, values []string, data TemplateData) ([]string, error) {
	out := make([]string, len(values))
	for i, v := range values {
		rendered, err := Render(fmt.Sprintf("%s[%d]", name, i), v, data)
		if err != nil {
			return nil, err
		}
		out[i] = rendered
	}
	return out, nil
}
