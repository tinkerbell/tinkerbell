package flatfile

import (
	"errors"
	"io"
	"os"

	"gopkg.in/yaml.v2"
)

// FromYAML constructs a new Backend using data from r to define instances. r should provide raw
// YAML data.
func FromYAML(r io.Reader) (*Backend, error) {
	var instances []Instance
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&instances); err != nil {
		return nil, err
	}

	return NewBackend(instances), nil
}

// FromYAMLFile constructs a new Backend using data from the YAML file at path.
func FromYAMLFile(path string) (*Backend, error) {
	if path == "" {
		return nil, errors.New("flatfile: path cannot be empty")
	}

	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	return FromYAML(fh)
}
