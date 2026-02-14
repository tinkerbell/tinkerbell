package webhttp

import (
	"errors"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "unauthorized error",
			err:      errors.New("Unauthorized: token expired"),
			expected: true,
		},
		{
			name:     "forbidden error - NOT an auth error",
			err:      errors.New("Forbidden: insufficient permissions"),
			expected: false, // Authorization errors should not trigger logout
		},
		{
			name:     "certificate error",
			err:      errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			expected: true,
		},
		{
			name:     "token error",
			err:      errors.New("invalid bearer token"),
			expected: true,
		},
		{
			name:     "expired token",
			err:      errors.New("token has expired"),
			expected: true,
		},
		{
			name:     "provide credentials error",
			err:      errors.New("must provide credentials"),
			expected: true,
		},
		{
			name:     "authentication failed",
			err:      errors.New("authentication failed"),
			expected: true,
		},
		{
			name:     "authorization failed - NOT an auth error",
			err:      errors.New("authorization check failed"),
			expected: false, // Authorization errors should not trigger logout
		},
		{
			name:     "unauthenticated error",
			err:      errors.New("unauthenticated request"),
			expected: true,
		},
		{
			name:     "not authenticated",
			err:      errors.New("user is not authenticated"),
			expected: true,
		},
		{
			name:     "non-auth error",
			err:      errors.New("failed to connect to database"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("resource not found"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthError(tt.err)
			if result != tt.expected {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
