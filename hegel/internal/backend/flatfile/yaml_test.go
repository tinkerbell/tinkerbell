package flatfile_test

import (
	"testing"

	. "github.com/tinkerbell/hegel/internal/backend/flatfile"
)

func TestFromYAMLFile(t *testing.T) {
	cases := []struct {
		Name        string
		Path        string
		ExpectError bool
	}{
		{
			Name: "ValidYAML",
			Path: "testdata/TestFromYAMLFile_Valid.yml",
		},
		{
			Name:        "InvalidYAML",
			Path:        "testdata/TestFromYAMLFile_Invalid.yml",
			ExpectError: true,
		},
		{
			Name:        "MissingYAMLFile",
			Path:        "testdata/TestFromYAMLFile_Missing.yml",
			ExpectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := FromYAMLFile(tc.Path)
			if tc.ExpectError {
				if err == nil {
					t.Fatal("Expected error but received nil")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected nil error; Received: %v", err)
				}
			}
		})
	}
}
