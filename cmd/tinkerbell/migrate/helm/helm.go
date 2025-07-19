package helm

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"os"
	"strings"

	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/migrate/helm/strvals"
	"gopkg.in/yaml.v3"
)

func NewCommand() *ff.Command {
	fs := ff.NewFlagSet("helm")
	vs := fs.StringList('s', "set", "set values on the command line")
	mt := fs.String('m', "migration-template", "", "path to the migration template file")
	previousValues := fs.String('p', "previous-values", "", "path to the 0.6.2 values.yaml file, if none is provided the command will read from stdin")
	return &ff.Command{
		Name:     "helm",
		Usage:    "helm [flags]",
		LongHelp: "Helm migration commands.",
		Flags:    fs,
		Exec: func(_ context.Context, _ []string) error {
			var input []byte
			if *previousValues != "" {
				// Read from the specified file
				i, err := os.ReadFile(*previousValues)
				if err != nil {
					return fmt.Errorf("error reading previous values file: %w", err)
				}
				input = i
			} else {
				// Read from stdin and store the input in a []byte slice
				i, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("error reading from stdin: %w", err)
				}
				input = i
			}
			// convert to map[string]interface{}
			var data map[string]interface{}
			if err := yaml.Unmarshal(input, &data); err != nil {
				return fmt.Errorf("error unmarshaling stdin yaml: %w", err)
			}

			// read in the migration template as a string
			mTemplate, err := os.ReadFile(*mt)
			if err != nil {
				return fmt.Errorf("error reading migration template: %w", err)
			}

			parsedTemplate, err := template.New("migration").Funcs(extraFuncs()).Parse(string(mTemplate))
			if err != nil {
				return fmt.Errorf("error parsing migration template: %w", err)
			}

			var renderedMigrationBuf bytes.Buffer
			err = parsedTemplate.Execute(&renderedMigrationBuf, data)
			if err != nil {
				return fmt.Errorf("error executing migration template: %w", err)
			}

			var migratedConfig map[string]interface{}
			err = yaml.Unmarshal(renderedMigrationBuf.Bytes(), &migratedConfig)
			if err != nil {
				return fmt.Errorf("error parsing migrated yaml values: %w", err)
			}

			for _, v := range *vs {
				if err := strvals.ParseInto(v, migratedConfig); err != nil {
					return fmt.Errorf("error parsing --set value %q: %w", v, err)
				}
			}

			// send the templated yaml to stdout
			// pretty print the yaml output
			yamlEncoder := yaml.NewEncoder(os.Stdout)
			yamlEncoder.SetIndent(2)
			if err := yamlEncoder.Encode(migratedConfig); err != nil {
				return fmt.Errorf("error encoding migrated yaml values: %w", err)
			}

			return nil
		},
	}
}

// Modified from https://github.com/helm/helm/blob/2feac15cc3252c97c997be2ced1ab8afe314b429/pkg/engine/funcs.go#L43
func extraFuncs() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")

	f["quoteEach"] = quoteEach
	f["toYaml"] = toYAML

	return f
}

func quoteEach(s []interface{}) (string, error) {
	var quoted []string
	for _, v := range s {
		str, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("quoteEach: expected all elements to be strings, got %T", v)
		}
		quoted = append(quoted, fmt.Sprintf("%q", str))
	}
	return strings.Join(quoted, ", "), nil
}

// toYAML takes an interface, marshals it to yaml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toYAML(v any) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}
