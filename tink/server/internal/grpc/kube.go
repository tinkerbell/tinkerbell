package grpc

import (
	"errors"
	"regexp"
	"strings"
)

var (
	errEmptyName          = errors.New("name cannot be empty")
	errMaxLenExceeded     = errors.New("name exceeds maximum length of 63 characters")
	errInvalidStartEnd    = errors.New("name cannot start or end with hyphen")
	errConsecutiveHyphens = errors.New("name contains consecutive hyphens")
	errInvalidChars       = errors.New("name contains invalid characters")
)

// makeValidName makes the name a valid Kubernetes object name.
// If a prefix is defined it will be prepended to the name.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
func makeValidName(name, prefix string) (string, error) {
	// Handle empty input
	if len(strings.TrimSpace(name)) == 0 {
		return "", errEmptyName
	}

	// Start with the original name
	result := strings.TrimSpace(name)
	if prefix != "" {
		// Prepend the prefix if provided
		result = prefix + result
	}

	// Ensure starts and ends with alphanumeric character
	if !regexp.MustCompile(`^[a-zA-Z0-9]$`).MatchString(result) {
		if !regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(result) {
			result = "e" + result
		}
		if !regexp.MustCompile(`[a-zA-Z0-9]$`).MatchString(result) {
			result += "e"
		}
	}

	// Replace invalid characters with hyphen
	result = regexp.MustCompile(`[^a-zA-Z0-9-]`).ReplaceAllString(result, "-")

	// Remove duplicate hyphens
	result = regexp.MustCompile(`-+`).ReplaceAllString(result, "-")

	// Remove leading/trailing hyphens
	result = strings.Trim(result, "-")

	// Convert to lowercase as required by RFC 1123
	result = strings.ToLower(result)

	// Length check and truncation
	if len(result) > 63 {
		result = result[:63]
	}

	// Verify final result meets all criteria
	if err := isValid(result); err != nil {
		return "", err
	}

	return result, nil
}

// isValid checks if a name meets all Kubernetes naming requirements
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
func isValid(name string) error {
	if len(name) == 0 {
		return errEmptyName
	}
	if len(name) > 63 {
		return errMaxLenExceeded
	}

	// Check for invalid patterns
	if matched, _ := regexp.MatchString(`^-|-+$`, name); matched {
		return errInvalidStartEnd
	}
	if matched, _ := regexp.MatchString(`--`, name); matched {
		return errConsecutiveHyphens
	}
	if matched, _ := regexp.MatchString(`[^a-z0-9-]`, name); matched {
		return errInvalidChars
	}

	return nil
}

// makeValidLabel makes the value of a Kubernetes label valid based off of https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
// Valid label value:
// must be 63 characters or less (can be empty),
// unless empty, must begin and end with an alphanumeric character ([a-z0-9A-Z]),
// could contain dashes (-), underscores (_), dots (.), and alphanumerics between.
func makeValidLabel(value string) (string, error) {
	// Handle empty input
	if len(strings.TrimSpace(value)) == 0 {
		return "", errEmptyName
	}

	// Start with the original value
	result := strings.TrimSpace(value)

	// Ensure starts and ends with alphanumeric character
	if !regexp.MustCompile(`^[a-zA-Z0-9]$`).MatchString(result) {
		if !regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(result) {
			result = "l" + result
		}
		if !regexp.MustCompile(`[a-zA-Z0-9]$`).MatchString(result) {
			result += "l"
		}
	}

	// Replace invalid characters with hyphen
	result = regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(result, "-")

	// Remove duplicate hyphens
	result = regexp.MustCompile(`-+`).ReplaceAllString(result, "-")

	// Remove leading/trailing hyphens
	result = strings.Trim(result, "-")

	// Convert to lowercase as required by RFC 1123
	// result = strings.ToLower(result)

	// Length check and truncation
	if len(result) > 63 {
		result = result[:63]
	}

	return result, nil
}
