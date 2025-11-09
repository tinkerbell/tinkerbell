package template

import (
	"bytes"
	"text/template"
)

// GenerateTemplate generates content from a template and data.
func GenerateTemplate(templateContent string, data any) ([]byte, error) {
	t := template.New("template")
	t, err := t.Parse(templateContent)
	if err != nil {
		return []byte{}, err
	}
	buffer := new(bytes.Buffer)
	if err := t.Execute(buffer, data); err != nil {
		return []byte{}, err
	}

	return buffer.Bytes(), nil
}
