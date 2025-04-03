package grpc

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMakeValid(t *testing.T) {
	testCases := map[string]struct {
		inputName     string
		inputPrefix   string
		expectedName  string
		expectedError error
	}{
		"spaces": {
			inputName:    "  my name  ",
			expectedName: "my-name",
		},
		"empty": {
			inputName:     "",
			expectedName:  "",
			expectedError: errEmptyName,
		},
		"truncated name": {
			inputName:    "very-long-name-that-exceeds-the-sixty-three-character-limit-and-needs-to-be-truncated",
			expectedName: "very-long-name-that-exceeds-the-sixty-three-character-limit-and",
		},
		"invalid characters": {
			inputName:    "invalid@name",
			expectedName: "invalid-name",
		},
		"invalid start": {
			inputName:    "-starts-with-hyphen",
			expectedName: "e-starts-with-hyphen",
		},
		"invalid end": {
			inputName:    "ends-with-hyphen-",
			expectedName: "ends-with-hyphen-e",
		},
		"multiple hyphens": {
			inputName:    "--multiple---hyphens--",
			expectedName: "e-multiple-hyphens-e",
		},
		"valid name": {
			inputName:    "my-app",
			expectedName: "my-app",
		},
		"valid name with prefix": {
			inputName:    "my-app",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-my-app",
		},
		"valid name with prefix and spaces": {
			inputName:    "  my app  ",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-my-app",
		},
		"valid name with prefix and invalid characters": {
			inputName:    "invalid@name",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-invalid-name",
		},
		"valid name with prefix and invalid start": {
			inputName:    "-starts-with-hyphen",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-starts-with-hyphen",
		},
		"valid name with prefix and invalid end": {
			inputName:    "ends-with-hyphen-",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-ends-with-hyphen-e",
		},
		"valid name with prefix and multiple hyphens": {
			inputName:    "--multiple---hyphens--",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-multiple-hyphens-e",
		},
		"valid name with prefix and truncated name": {
			inputName:    "very-long-name-that-exceeds-the-sixty-three-character-limit-and-needs-to-be-truncated",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-very-long-name-that-exceeds-the-sixty-three-characte",
		},
		"valid name with prefix and empty name": {
			inputName:     "",
			inputPrefix:   "enrollment-",
			expectedName:  "",
			expectedError: errEmptyName,
		},
		"valid name with prefix and empty name with spaces": {
			inputName:     "  ",
			inputPrefix:   "enrollment-",
			expectedName:  "",
			expectedError: errEmptyName,
		},
		"valid name with prefix and empty name with invalid characters": {
			inputName:    "invalid@name",
			inputPrefix:  "enrollment-",
			expectedName: "enrollment-invalid-name",
		},
		"valid name with invalid prefix start": {
			inputName:    "my-app",
			inputPrefix:  "-prefix",
			expectedName: "e-prefixmy-app",
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := makeValidName(test.inputName, test.inputPrefix)
			if !errors.Is(err, test.expectedError) {
				t.Errorf("expected errors don't match. got: %v want: %v", err, test.expectedError)
			}

			if diff := cmp.Diff(got, test.expectedName); diff != "" {
				t.Errorf("makeValid() = %v", diff)
			}

		})
	}
}
