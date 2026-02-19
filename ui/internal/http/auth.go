package webhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SessionDurationSeconds is the duration in seconds that auth cookies remain valid.
	// Set to 8 hours to reduce exposure window if credentials are compromised.
	SessionDurationSeconds       = 3600 * 8
	cookieNameAuthToken          = "auth_token"
	cookieNameAPIServer          = "auth_apiserver"
	cookieNameSANamespace        = "sa_namespace"
	cookieNameInsecureSkipVerify = "insecure_skip_verify"
	// Display-only cookies (non-HttpOnly) for UI display purposes only.
	// These are NOT used for server-side logic - only for showing info in the browser.
	//nolint:gosec // This is a field name, not a secret.
	cookieNameTokenExpiry        = "token_expiry"         // Token expiration timestamp
	cookieNameAPIServerDisplay   = "apiserver_display"    // API server URL for menu display
	cookieNameSANamespaceDisplay = "sa_namespace_display" // Service account namespace for UI
)

// HandleLogin renders the login page.
func HandleLogin(c *gin.Context, log logr.Logger) {
	baseURL := GetBaseURL(c)
	component := templates.LoginPage(baseURL)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleLoginValidate validates the service token and API server URL.
// It performs comprehensive validation including URL format checking, token verification,
// and permission validation before setting secure authentication cookies.
func HandleLoginValidate(c *gin.Context, log logr.Logger) {
	token := c.PostForm("token")
	if token == "" {
		c.JSON(403, gin.H{"error": "Service account token is required"})
		return
	}

	apiServer := c.PostForm("apiServer")
	if apiServer == "" {
		c.JSON(403, gin.H{"error": "Kubernetes API server URL is required"})
		return
	}

	token = strings.TrimSpace(token)
	apiServer = strings.TrimSpace(apiServer)

	// Validate API server URL format and security (SSRF protection)
	if err := validateAPIServerURL(apiServer); err != nil {
		log.Error(err, "Invalid API server URL",
			"url", apiServer,
			"clientIP", c.ClientIP(),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid API server URL. Must be a valid HTTPS URL.",
		})
		return
	}

	// Token validation is delegated to Kubernetes API server

	// Get TLS verification setting from form (checkbox)
	insecureSkipVerifyStr := c.PostForm("insecureSkipVerify")
	insecureSkipVerify := insecureSkipVerifyStr == "true"

	// Log security warning when TLS verification is disabled
	if insecureSkipVerify {
		log.V(1).Info("TLS certificate verification disabled for Kubernetes API connection",
			"apiServer", apiServer,
			"clientIP", c.ClientIP(),
			"userAgent", c.GetHeader("User-Agent"),
		)
	}

	testClient, err := NewKubeClientFromTokenAndServer(token, apiServer, insecureSkipVerify)
	if err != nil {
		c.JSON(403, gin.H{"error": fmt.Sprintf("Failed to create Kubernetes client: %v", err)})
		return
	}

	ctx := c.Request.Context()
	// Extract namespace from token for namespace-scoped permission check
	saNamespace := extractNamespaceFromToken(token)

	// Validate token by checking permissions instead of listing namespaces.
	// First try cluster-wide access, then fall back to namespace-scoped if token has a namespace.
	err = validateTokenPermissions(ctx, testClient, "")
	if err != nil && saNamespace != "" {
		// Try namespace-scoped permission check
		err = validateTokenPermissions(ctx, testClient, saNamespace)
	}
	if err != nil {
		// Provide user-friendly error messages for common authentication issues
		errMsg := err.Error()
		var userMsg string

		switch {
		case strings.Contains(errMsg, "provide credentials") || strings.Contains(errMsg, "Unauthorized"):
			userMsg = "The provided token is invalid or has expired. Generate a new token with: kubectl create token <service-account-name>"
		case strings.Contains(errMsg, "Forbidden") || strings.Contains(errMsg, "forbidden"):
			userMsg = "The token is valid but has no permissions. Contact your cluster administrator to grant necessary RBAC permissions."
		case strings.Contains(errMsg, "connect") || strings.Contains(errMsg, "connection refused"):
			userMsg = "Unable to connect to the Kubernetes API server. Please verify the API server URL and ensure the server is accessible."
		case strings.Contains(errMsg, "certificate") || strings.Contains(errMsg, "tls"):
			userMsg = "TLS certificate verification failed. Try enabling 'Skip TLS verification' if using self-signed certificates."
		default:
			userMsg = "An error occurred while validating the token. Please check your credentials."
		}

		// Log full error details server-side for debugging
		log.Error(err, "Login validation failed",
			"clientIP", c.ClientIP(),
			"apiServer", apiServer,
			"userAgent", c.GetHeader("User-Agent"),
		)

		// Note: errMsg is not HTML-escaped here because it's sent as JSON and
		// rendered via JavaScript's .textContent which safely handles special characters.
		c.JSON(403, gin.H{
			"error":   userMsg,
			"details": errMsg,
		})
		return
	}

	encodedToken := base64.StdEncoding.EncodeToString([]byte(token))
	encodedAPIServer := base64.StdEncoding.EncodeToString([]byte(apiServer))
	encodedNamespace := ""
	if saNamespace != "" {
		encodedNamespace = base64.StdEncoding.EncodeToString([]byte(saNamespace))
	}

	// Set Secure flag based on TLS detection
	// In production, cookies should always be Secure. In development/testing without HTTPS,
	// the secure flag is conditionally set to allow local testing.
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

	// Set SameSite to Strict for CSRF protection
	c.SetSameSite(http.SameSiteStrictMode)

	// Use reduced session duration (8 hours instead of 7 days)
	// auth_token and auth_apiserver are HttpOnly for security (prevent XSS token theft)
	c.SetCookie(cookieNameAuthToken, encodedToken, SessionDurationSeconds, "/", "", secure, true)
	c.SetCookie(cookieNameAPIServer, encodedAPIServer, SessionDurationSeconds, "/", "", secure, true)
	if encodedNamespace != "" {
		// sa_namespace is HttpOnly for security
		c.SetCookie(cookieNameSANamespace, encodedNamespace, SessionDurationSeconds, "/", "", secure, true)
		// sa_namespace_display is for JS to auto-select namespace in UI
		c.SetCookie(cookieNameSANamespaceDisplay, saNamespace, SessionDurationSeconds, "/", "", secure, false)
	}

	// Extract token expiry from JWT and set as non-HttpOnly cookie for UI display
	if expiry := extractTokenExpiry(token); expiry != "" {
		c.SetCookie(cookieNameTokenExpiry, expiry, SessionDurationSeconds, "/", "", secure, false)
	}
	// Set API server URL as non-HttpOnly cookie for UI display only.
	// Server-side logic uses the HttpOnly auth_apiserver cookie, not this one.
	c.SetCookie(cookieNameAPIServerDisplay, apiServer, SessionDurationSeconds, "/", "", secure, false)
	// Note: insecure_skip_verify is stored in an HttpOnly cookie to prevent JavaScript tampering.
	// Server-side validation ensures only "true" or empty values are accepted.
	// WARNING: Disabling TLS verification exposes connections to man-in-the-middle attacks.
	c.SetCookie(cookieNameInsecureSkipVerify, insecureSkipVerifyStr, SessionDurationSeconds, "/", "", secure, true)

	c.JSON(200, gin.H{
		"success": true,
		"message": "Token validated successfully",
	})
}

// validateAPIServerURL validates that the API server URL is properly formatted and secure.
// It prevents SSRF attacks by ensuring the URL uses HTTPS and has proper structure.
func validateAPIServerURL(apiServer string) error {
	if apiServer == "" {
		return fmt.Errorf("API server URL is required")
	}

	// Parse the URL
	u, err := url.Parse(apiServer)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Must use HTTPS for security
	if u.Scheme != "https" {
		return fmt.Errorf("API server must use HTTPS protocol, got: %s", u.Scheme)
	}

	// Validate hostname is present
	if u.Hostname() == "" {
		return fmt.Errorf("API server URL must include a hostname")
	}

	// Note: We don't block private IPs here as many Kubernetes clusters
	// run on private networks (e.g., kind, minikube, internal clusters).
	// In production deployments, network policies should restrict access appropriately.

	return nil
}

// extractTokenExpiry extracts the expiry timestamp from a JWT token.
// Returns the Unix timestamp as a string, or empty string if parsing fails.
func extractTokenExpiry(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	// Decode the payload (second part), handling base64url encoding
	payload := parts[1]
	// Add padding if needed
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	// Replace base64url chars with standard base64
	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	if claims.Exp == 0 {
		return ""
	}

	return fmt.Sprintf("%d", claims.Exp)
}

// HandleLogout logs out the user by clearing all authentication cookies.
func HandleLogout(c *gin.Context) {
	// Set Secure flag based on TLS detection
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

	// Set SameSite for security consistency
	c.SetSameSite(http.SameSiteStrictMode)

	// Clear all authentication cookies by setting MaxAge to -1
	c.SetCookie(cookieNameAuthToken, "", -1, "/", "", secure, true)
	c.SetCookie(cookieNameAPIServer, "", -1, "/", "", secure, true)
	c.SetCookie(cookieNameSANamespace, "", -1, "/", "", secure, true)
	c.SetCookie(cookieNameInsecureSkipVerify, "", -1, "/", "", secure, true)
	c.SetCookie(cookieNameTokenExpiry, "", -1, "/", "", secure, false)
	c.SetCookie(cookieNameAPIServerDisplay, "", -1, "/", "", secure, false)
	c.SetCookie(cookieNameSANamespaceDisplay, "", -1, "/", "", secure, false)

	c.JSON(200, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

// GetTokenAndAPIServerFromRequest retrieves and decodes token, API server, and TLS verification setting from request.
func GetTokenAndAPIServerFromRequest(c *gin.Context) (string, string, bool) {
	var token, apiServer string
	var insecureSkipVerify bool

	if tokenCookie, err := c.Cookie(cookieNameAuthToken); err == nil && tokenCookie != "" {
		if decoded, err := base64.StdEncoding.DecodeString(tokenCookie); err == nil {
			token = string(decoded)
		}
	}

	if apiServerCookie, err := c.Cookie(cookieNameAPIServer); err == nil && apiServerCookie != "" {
		if decoded, err := base64.StdEncoding.DecodeString(apiServerCookie); err == nil {
			apiServer = string(decoded)
		}
	}

	// Get TLS verification setting with validation, default to false (verify TLS)
	// Only accept "true" as a valid value; any other value defaults to secure (false)
	if insecureSkipVerifyCookie, err := c.Cookie(cookieNameInsecureSkipVerify); err == nil {
		// Server-side validation: only accept "true" to prevent tampering
		if insecureSkipVerifyCookie == "true" {
			insecureSkipVerify = true
		}
		// Any other value (including malformed values) defaults to false (secure)
	}

	return token, apiServer, insecureSkipVerify
}

// AuthMiddleware checks if user has a valid token before accessing protected routes.
func AuthMiddleware(log logr.Logger, baseURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, apiServer, insecureSkipVerify := GetTokenAndAPIServerFromRequest(c)
		if token == "" || apiServer == "" {
			c.Redirect(302, filepath.Join(baseURL, "login"))
			c.Abort()
			return
		}

		// Log audit trail when TLS verification is disabled
		if insecureSkipVerify {
			log.V(1).Info("Request using connection with TLS verification disabled",
				"path", c.Request.URL.Path,
				"apiServer", apiServer,
				"clientIP", c.ClientIP(),
			)
		}

		userClient, err := NewKubeClientFromTokenAndServer(token, apiServer, insecureSkipVerify)
		if err != nil {
			log.Error(err, "Failed to create client", "path", c.Request.URL.Path)
			c.Redirect(302, filepath.Join(baseURL, "login"))
			c.Abort()
			return
		}

		// Extract and store service account namespace in context
		saNamespace := extractNamespaceFromToken(token)
		if saNamespace != "" {
			c.Set(cookieNameSANamespace, saNamespace)
		}

		c.Set("kubeClient", userClient)
		c.Next()
	}
}

// AutoLoginMiddleware injects a pre-configured KubeClient into the request context.
// Used when auto-login is enabled to bypass cookie-based authentication.
// When namespace is non-empty it is stored as the service-account namespace so
// that downstream handlers can fall back to namespace-scoped queries.
func AutoLoginMiddleware(client *KubeClient, namespace string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if namespace != "" {
			c.Set(cookieNameSANamespace, namespace)
		}
		c.Set("kubeClient", client)
		c.Next()
	}
}

// validateTokenPermissions validates that a token has permissions to list Hardware objects.
// This works for both cluster-wide and namespace-scoped service accounts by using
// SelfSubjectAccessReview to check if the token can list Hardware resources.
// If namespace is provided, checks namespace-scoped permission; otherwise checks cluster-wide.
// kubectl auth can-i list hardware.tinkerbell.org [--namespace <ns>]
func validateTokenPermissions(ctx context.Context, client *KubeClient, namespace string) error {
	if client == nil || client.clientset == nil {
		return fmt.Errorf("invalid client")
	}

	// Use SelfSubjectAccessReview to check if the token can list Hardware objects.
	// This approach works regardless of whether any Hardware objects exist in the cluster.
	sar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:      "list",
				Group:     "tinkerbell.org",
				Resource:  "hardware",
				Namespace: namespace, // Empty string means cluster-wide
			},
		},
	}

	result, err := client.clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !result.Status.Allowed {
		// If cluster-wide check failed, try with the service account's namespace
		if namespace == "" {
			return fmt.Errorf("token has no permissions to list Tinkerbell Hardware objects. Contact your cluster administrator to grant necessary RBAC permissions")
		}
		return fmt.Errorf("token has no permissions to list Tinkerbell Hardware objects in namespace %s. Contact your cluster administrator to grant necessary RBAC permissions", namespace)
	}

	return nil
}

// extractNamespaceFromToken extracts the service account namespace from a Kubernetes JWT token.
func extractNamespaceFromToken(token string) string {
	// JWT tokens have 3 parts separated by dots: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	// Parse the JSON payload
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}

	// Extract the namespace from nested kubernetes.io.namespace claim
	if k8sio, ok := claims["kubernetes.io"].(map[string]interface{}); ok {
		if ns, ok := k8sio["namespace"].(string); ok {
			return ns
		}
	}

	return ""
}
