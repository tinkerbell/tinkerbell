package webhttp

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGetTokenAndAPIServerFromRequest_Cookies(t *testing.T) {
	tests := []struct {
		name          string
		tokenCookie   string
		apiCookie     string
		wantToken     string
		wantAPIServer string
	}{
		{
			name:          "valid cookies",
			tokenCookie:   base64.StdEncoding.EncodeToString([]byte("my-jwt-token")),
			apiCookie:     base64.StdEncoding.EncodeToString([]byte("https://kube.example.com:6443")),
			wantToken:     "my-jwt-token",
			wantAPIServer: "https://kube.example.com:6443",
		},
		{
			name:          "empty cookies",
			tokenCookie:   "",
			apiCookie:     "",
			wantToken:     "",
			wantAPIServer: "",
		},
		{
			name:          "invalid base64 in token cookie",
			tokenCookie:   "not-valid-base64!!!",
			apiCookie:     base64.StdEncoding.EncodeToString([]byte("https://kube.example.com:6443")),
			wantToken:     "",
			wantAPIServer: "https://kube.example.com:6443",
		},
		{
			name:          "invalid base64 in apiserver cookie",
			tokenCookie:   base64.StdEncoding.EncodeToString([]byte("my-jwt-token")),
			apiCookie:     "not-valid-base64!!!",
			wantToken:     "my-jwt-token",
			wantAPIServer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.tokenCookie != "" {
				c.Request.AddCookie(&http.Cookie{Name: "auth_token", Value: tt.tokenCookie})
			}
			if tt.apiCookie != "" {
				c.Request.AddCookie(&http.Cookie{Name: "auth_apiserver", Value: tt.apiCookie})
			}

			gotToken, gotAPIServer, _ := GetTokenAndAPIServerFromRequest(c)

			if gotToken != tt.wantToken {
				t.Errorf("token = %q, want %q", gotToken, tt.wantToken)
			}
			if gotAPIServer != tt.wantAPIServer {
				t.Errorf("apiServer = %q, want %q", gotAPIServer, tt.wantAPIServer)
			}
		})
	}
}

func TestHandleLogout(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/logout", nil)

	HandleLogout(c)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Check cookies are cleared (set with max-age -1)
	cookies := w.Result().Cookies()
	foundAuthToken := false
	foundAuthAPIServer := false

	for _, cookie := range cookies {
		if cookie.Name == "auth_token" {
			foundAuthToken = true
			if cookie.MaxAge != -1 {
				t.Errorf("auth_token cookie MaxAge = %d, want -1", cookie.MaxAge)
			}
		}
		if cookie.Name == "auth_apiserver" {
			foundAuthAPIServer = true
			if cookie.MaxAge != -1 {
				t.Errorf("auth_apiserver cookie MaxAge = %d, want -1", cookie.MaxAge)
			}
		}
	}

	if !foundAuthToken {
		t.Error("auth_token cookie not found in response")
	}
	if !foundAuthAPIServer {
		t.Error("auth_apiserver cookie not found in response")
	}
}

func TestHandleLoginValidate_EmptyToken(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/login/validate", nil)
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	HandleLoginValidate(c, logr.Discard())

	if w.Code != http.StatusForbidden {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleLoginValidate_EmptyAPIServer(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/login/validate", nil)
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.PostForm = map[string][]string{
		"token": {"some-token"},
	}

	HandleLoginValidate(c, logr.Discard())

	if w.Code != http.StatusForbidden {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleLoginValidate_InvalidTokenFormat(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/login/validate", nil)
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.PostForm = map[string][]string{
		"token":     {"invalid-token-no-dots"},
		"apiServer": {"https://kube.example.com"},
	}

	HandleLoginValidate(c, logr.Discard())

	if w.Code != http.StatusForbidden {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestValidateAPIServerURL(t *testing.T) {
	tests := []struct {
		name      string
		apiServer string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid https url",
			apiServer: "https://kube.example.com:6443",
			wantErr:   false,
		},
		{
			name:      "valid https url without port",
			apiServer: "https://kube.example.com",
			wantErr:   false,
		},
		{
			name:      "valid https url with path",
			apiServer: "https://kube.example.com:6443/api",
			wantErr:   false,
		},
		{
			name:      "empty url",
			apiServer: "",
			wantErr:   true,
			errMsg:    "required",
		},
		{
			name:      "http url not allowed",
			apiServer: "http://kube.example.com:6443",
			wantErr:   true,
			errMsg:    "HTTPS",
		},
		{
			name:      "missing scheme",
			apiServer: "kube.example.com:6443",
			wantErr:   true,
			errMsg:    "HTTPS",
		},
		{
			name:      "invalid url format",
			apiServer: "://invalid",
			wantErr:   true,
			errMsg:    "invalid",
		},
		{
			name:      "ftp scheme not allowed",
			apiServer: "ftp://kube.example.com",
			wantErr:   true,
			errMsg:    "HTTPS",
		},
		{
			name:      "missing hostname",
			apiServer: "https://",
			wantErr:   true,
			errMsg:    "hostname",
		},
		{
			name:      "localhost allowed",
			apiServer: "https://localhost:6443",
			wantErr:   false,
		},
		{
			name:      "private ip allowed",
			apiServer: "https://192.168.1.100:6443",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAPIServerURL(tt.apiServer)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAPIServerURL() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAPIServerURL() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validateAPIServerURL() unexpected error = %v", err)
			}
		})
	}
}

func TestExtractNamespaceFromToken(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		wantNS string
	}{
		{
			name:   "empty token",
			token:  "",
			wantNS: "",
		},
		{
			name:   "invalid token format - no dots",
			token:  "invalidtoken",
			wantNS: "",
		},
		{
			name:   "invalid token format - only two parts",
			token:  "header.payload",
			wantNS: "",
		},
		{
			name:   "invalid base64 in payload",
			token:  "header.!!!invalid!!!.signature",
			wantNS: "",
		},
		{
			name:   "valid jwt without namespace claim",
			token:  "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test"}`)) + ".signature",
			wantNS: "",
		},
		{
			name:   "valid jwt with namespace in kubernetes.io claim",
			token:  "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(`{"kubernetes.io":{"namespace":"tinkerbell"}}`)) + ".signature",
			wantNS: "tinkerbell",
		},
		{
			name:   "valid jwt with default namespace",
			token:  "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(`{"kubernetes.io":{"namespace":"default"}}`)) + ".signature",
			wantNS: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNamespaceFromToken(tt.token)
			if got != tt.wantNS {
				t.Errorf("extractNamespaceFromToken() = %q, want %q", got, tt.wantNS)
			}
		})
	}
}

func TestHandleLoginValidate_InvalidAPIServerURL(t *testing.T) {
	tests := []struct {
		name       string
		apiServer  string
		wantStatus int
	}{
		{
			name:       "http not allowed",
			apiServer:  "http://kube.example.com",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid url",
			apiServer:  "not-a-url",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use unique IP to avoid rate limiting
			testIP := "api-server-test-" + tt.name

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/login/validate", nil)
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			c.Request.Header.Set("X-Forwarded-For", testIP)
			c.Request.PostForm = map[string][]string{
				"token":     {"some-token"},
				"apiServer": {tt.apiServer},
			}

			HandleLoginValidate(c, logr.Discard())

			if w.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleLogin(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/login", nil)

	HandleLogin(c, logr.Discard())

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("Content-Type = %q, want %q", contentType, "text/html")
	}
}

func TestGetTokenAndAPIServerFromRequest_InsecureSkipVerify(t *testing.T) {
	tests := []struct {
		name             string
		cookieValue      string
		wantInsecureSkip bool
	}{
		{
			name:             "true value",
			cookieValue:      "true",
			wantInsecureSkip: true,
		},
		{
			name:             "false value",
			cookieValue:      "false",
			wantInsecureSkip: false,
		},
		{
			name:             "empty value",
			cookieValue:      "",
			wantInsecureSkip: false,
		},
		{
			name:             "invalid value defaults to false",
			cookieValue:      "invalid",
			wantInsecureSkip: false,
		},
		{
			name:             "TRUE uppercase defaults to false",
			cookieValue:      "TRUE",
			wantInsecureSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.cookieValue != "" {
				c.Request.AddCookie(&http.Cookie{Name: "insecure_skip_verify", Value: tt.cookieValue})
			}

			_, _, gotInsecureSkip := GetTokenAndAPIServerFromRequest(c)

			if gotInsecureSkip != tt.wantInsecureSkip {
				t.Errorf("insecureSkipVerify = %v, want %v", gotInsecureSkip, tt.wantInsecureSkip)
			}
		})
	}
}

func TestHandleLogout_ClearsAllCookies(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/logout", nil)

	HandleLogout(c)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Check all auth cookies are cleared
	expectedCookies := map[string]bool{
		"auth_token":           false,
		"auth_apiserver":       false,
		"sa_namespace":         false,
		"insecure_skip_verify": false,
	}

	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		if _, exists := expectedCookies[cookie.Name]; exists {
			expectedCookies[cookie.Name] = true
			if cookie.MaxAge != -1 {
				t.Errorf("%s cookie MaxAge = %d, want -1", cookie.Name, cookie.MaxAge)
			}
		}
	}

	for name, found := range expectedCookies {
		if !found {
			t.Errorf("%s cookie not found in response", name)
		}
	}
}
