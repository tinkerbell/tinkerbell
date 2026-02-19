// Package ui provides the Tinkerbell web UI service.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/ui/assets"
	webhttp "github.com/tinkerbell/tinkerbell/ui/internal/http"
	"k8s.io/client-go/rest"
)

const (
	// DefaultBindAddr is the default IP address to bind the HTTP server to.
	DefaultBindAddr = "0.0.0.0"
	// DefaultBindPort is the default port for the web UI HTTP server.
	DefaultBindPort = 8085

	// HTTP server timeouts (QUAL-4)
	// HTTPReadTimeout is the maximum duration for reading the entire request.
	HTTPReadTimeout = 30 * time.Second
	// HTTPReadHeaderTimeout is the maximum duration for reading request headers.
	HTTPReadHeaderTimeout = 10 * time.Second
	// HTTPWriteTimeout is the maximum duration before timing out writes of the response.
	HTTPWriteTimeout = 30 * time.Second
	// HTTPIdleTimeout is the maximum duration for keep-alive connections.
	HTTPIdleTimeout = 120 * time.Second
	// HTTPShutdownTimeout is the maximum duration for graceful shutdown.
	HTTPShutdownTimeout = 30 * time.Second
	// HTTPMaxHeaderBytes is the maximum size of request headers.
	HTTPMaxHeaderBytes = 1 << 20 // 1 MB

	// StaticAssetCacheDuration is the cache duration for static assets (24 hours).
	StaticAssetCacheDuration = 24 * time.Hour

	// DefaultURLPrefix is the default URI path prefix for all web UI routes.
	DefaultURLPrefix = "/ui"
)

// Config holds the configuration for the web UI service.
type Config struct {
	BindAddr    string
	BindPort    int
	DebugMode   bool
	TLSCertFile string
	TLSKeyFile  string
	URLPrefix   string
	// EnableAutoLogin bypasses the login page and uses AutoLoginRestConfig for all requests.
	EnableAutoLogin bool
	// AutoLoginRestConfig is the Kubernetes REST config used when EnableAutoLogin is true.
	AutoLoginRestConfig *rest.Config
	// AutoLoginNamespace is the namespace to use for namespace-scoped fallbacks when EnableAutoLogin is true.
	AutoLoginNamespace string
}

type Option func(*Config)

func WithBindPort(port int) Option {
	return func(c *Config) {
		c.BindPort = port
	}
}

func WithURLPrefix(prefix string) Option {
	return func(c *Config) {
		c.URLPrefix = prefix
	}
}

// NewConfig creates a new Config with defaults merged with the provided config.
func NewConfig(opts ...Option) *Config {
	dc := &Config{
		BindAddr:    DefaultBindAddr,
		BindPort:    DefaultBindPort,
		DebugMode:   false,
		TLSCertFile: "",
		TLSKeyFile:  "",
		URLPrefix:   DefaultURLPrefix,
	}

	for _, opt := range opts {
		opt(dc)
	}

	return dc
}

// staticCacheMiddleware returns a Gin middleware that sets Cache-Control headers for static assets.
func staticCacheMiddleware() gin.HandlerFunc {
	cacheControl := fmt.Sprintf("public, max-age=%d", int(StaticAssetCacheDuration.Seconds()))
	return func(c *gin.Context) {
		c.Header("Cache-Control", cacheControl)
		c.Next()
	}
}

// securityHeadersMiddleware returns a Gin middleware that sets security headers on all responses.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		// Control referrer information
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// Permissive CSP that allows inline scripts (needed for templ templates)
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'")
		// HSTS for HTTPS connections
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// Start starts the web UI HTTP server.
func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	if !c.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(securityHeadersMiddleware())
	base := r.Group(c.URLPrefix)

	// Normalize URLPrefix for template URL generation.
	// When URLPrefix is "/", we store empty string to avoid double-slash issues
	// in URL concatenation (e.g., "/" + "/css/output.css" = "//css/output.css"
	// would be interpreted as a protocol-relative URL).
	templateBaseURL := c.URLPrefix
	if templateBaseURL == "/" {
		templateBaseURL = ""
	}

	// Set baseURL in context for all routes under base
	base.Use(func(gc *gin.Context) {
		gc.Set(webhttp.ContextKeyBaseURL, templateBaseURL)
		gc.Next()
	})

	// Create sub-filesystems for artwork and css from the embedded assets
	artworkFS, err := fs.Sub(assets.Artwork, "artwork")
	if err != nil {
		return err
	}
	cssFS, err := fs.Sub(assets.CSS, "css")
	if err != nil {
		return err
	}
	jsFS, err := fs.Sub(assets.JS, "js")
	if err != nil {
		return err
	}
	fontsFS, err := fs.Sub(assets.Fonts, "fonts")
	if err != nil {
		return err
	}

	// Serve embedded static files with cache headers
	staticCache := staticCacheMiddleware()

	artworkGroup := base.Group("/artwork")
	artworkGroup.Use(staticCache)
	artworkGroup.StaticFS("", http.FS(artworkFS))

	cssGroup := base.Group("/css")
	cssGroup.Use(staticCache)
	cssGroup.StaticFS("", http.FS(cssFS))

	jsGroup := base.Group("/js")
	jsGroup.Use(staticCache)
	jsGroup.StaticFS("", http.FS(jsFS))

	fontsGroup := base.Group("/fonts")
	fontsGroup.Use(staticCache)
	fontsGroup.StaticFS("", http.FS(fontsFS))

	// Also serve static files from BMC subdirectory paths to handle relative path requests
	bmcArtworkGroup := base.Group("/bmc/artwork")
	bmcArtworkGroup.Use(staticCache)
	bmcArtworkGroup.StaticFS("", http.FS(artworkFS))

	bmcCSSGroup := base.Group("/bmc/css")
	bmcCSSGroup.Use(staticCache)
	bmcCSSGroup.StaticFS("", http.FS(cssFS))

	bmcJSGroup := base.Group("/bmc/js")
	bmcJSGroup.Use(staticCache)
	bmcJSGroup.StaticFS("", http.FS(jsFS))

	bmcFontsGroup := base.Group("/bmc/fonts")
	bmcFontsGroup.Use(staticCache)
	bmcFontsGroup.StaticFS("", http.FS(fontsFS))

	// Favicon routes
	base.GET("/favicon.ico", func(c *gin.Context) {
		c.Header("Content-Type", "image/svg+xml")
		data, err := assets.Artwork.ReadFile("artwork/Tinkerbell-Icon-Dark.svg")
		if err != nil {
			log.Error(err, "Failed to read favicon file")
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/svg+xml", data)
	})
	base.GET("/favicon.svg", func(c *gin.Context) {
		c.Header("Content-Type", "image/svg+xml")
		data, err := assets.Artwork.ReadFile("artwork/Tinkerbell-Icon-Dark.svg")
		if err != nil {
			log.Error(err, "Failed to read favicon file")
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/svg+xml", data)
	})

	// Auth routes
	if c.EnableAutoLogin {
		// Auto-login mode: bypass login page, redirect to dashboard
		base.GET("/login", func(gc *gin.Context) {
			gc.Redirect(http.StatusFound, path.Join(c.URLPrefix, "/"))
		})
		base.POST("/api/auth/login", func(gc *gin.Context) {
			gc.JSON(http.StatusForbidden, gin.H{"error": "auto-login enabled, manual login is disabled"})
		})
		base.POST("/api/auth/logout", func(gc *gin.Context) {
			gc.Redirect(http.StatusFound, path.Join(c.URLPrefix, "/"))
		})
	} else {
		// Standard login mode: users authenticate via the login page
		base.GET("/login", func(gc *gin.Context) {
			webhttp.HandleLogin(gc, log)
		})
		base.POST("/api/auth/login", func(gc *gin.Context) {
			webhttp.HandleLoginValidate(gc, log)
		})
		base.POST("/api/auth/logout", webhttp.HandleLogout)
	}

	// Health check endpoints (QUAL-5)
	base.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"service":   "tinkerbell-web-ui",
			"timestamp": time.Now().Unix(),
		})
	})

	base.GET("/ready", func(c *gin.Context) {
		// Simple readiness check - server is ready if it can respond
		// More sophisticated checks could verify K8s API connectivity
		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"service":   "tinkerbell-web-ui",
			"timestamp": time.Now().Unix(),
		})
	})

	// Protected routes (require valid kubeconfig)
	protected := base.Group("/")
	if c.EnableAutoLogin {
		autoClient, err := webhttp.NewKubeClientFromRestConfig(c.AutoLoginRestConfig)
		if err != nil {
			return fmt.Errorf("failed to create auto-login kube client: %w", err)
		}
		protected.Use(webhttp.AutoLoginMiddleware(autoClient, c.AutoLoginNamespace))
	} else {
		protected.Use(webhttp.AuthMiddleware(log, c.URLPrefix))
	}
	{
		// Home page (dashboard / CRD browser)
		protected.GET("/", func(c *gin.Context) {
			webhttp.HandleDashboard(c, log)
		})

		// Hardware routes
		protected.GET("/hardware", func(c *gin.Context) {
			webhttp.HandleHardwareList(c, log)
		})
		protected.GET("/hardware-data", func(c *gin.Context) {
			webhttp.HandleHardwareData(c, log)
		})
		protected.GET("/hardware/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleHardwareDetail(c, log)
		})

		// Workflow routes
		protected.GET("/workflows", func(c *gin.Context) {
			webhttp.HandleWorkflowList(c, log)
		})
		protected.GET("/workflows-data", func(c *gin.Context) {
			webhttp.HandleWorkflowData(c, log)
		})
		protected.GET("/workflows/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleWorkflowDetail(c, log)
		})

		// Template routes
		protected.GET("/templates", func(c *gin.Context) {
			webhttp.HandleTemplateList(c, log)
		})
		protected.GET("/templates-data", func(c *gin.Context) {
			webhttp.HandleTemplateData(c, log)
		})
		protected.GET("/templates/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleTemplateDetail(c, log)
		})

		// WorkflowRuleSet routes
		protected.GET("/workflows/rulesets", func(c *gin.Context) {
			webhttp.HandleWorkflowRuleSetList(c, log)
		})
		protected.GET("/workflows/rulesets-data", func(c *gin.Context) {
			webhttp.HandleWorkflowRuleSetData(c, log)
		})
		protected.GET("/workflows/rulesets/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleWorkflowRuleSetDetail(c, log)
		})

		// BMC Machine routes
		protected.GET("/bmc/machines", func(c *gin.Context) {
			webhttp.HandleBMCMachineList(c, log)
		})
		protected.GET("/bmc/machines-data", func(c *gin.Context) {
			webhttp.HandleBMCMachineData(c, log)
		})
		protected.GET("/bmc/machines/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleBMCMachineDetail(c, log)
		})

		// BMC Job routes
		protected.GET("/bmc/jobs", func(c *gin.Context) {
			webhttp.HandleBMCJobList(c, log)
		})
		protected.GET("/bmc/jobs-data", func(c *gin.Context) {
			webhttp.HandleBMCJobData(c, log)
		})
		protected.GET("/bmc/jobs/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleBMCJobDetail(c, log)
		})

		// BMC Task routes
		protected.GET("/bmc/tasks", func(c *gin.Context) {
			webhttp.HandleBMCTaskList(c, log)
		})
		protected.GET("/bmc/tasks-data", func(c *gin.Context) {
			webhttp.HandleBMCTaskData(c, log)
		})
		protected.GET("/bmc/tasks/:namespace/:name", func(c *gin.Context) {
			webhttp.HandleBMCTaskDetail(c, log)
		})

		// Global search API
		protected.GET("/api/search", func(c *gin.Context) {
			webhttp.HandleGlobalSearch(c, log)
		})

		// Permissions page
		protected.GET("/permissions", func(c *gin.Context) {
			webhttp.HandlePermissions(c, log)
		})

		// Permission check endpoint for HTMX progressive loading
		protected.GET("/permissions/check/:resource", func(c *gin.Context) {
			webhttp.HandlePermissionCheck(c, log)
		})
	}

	addr := fmt.Sprintf("%s:%d", c.BindAddr, c.BindPort)
	log.Info("starting web UI server", "addr", addr, "tlsEnabled", c.TLSCertFile != "")

	// Configure HTTP server with comprehensive timeouts (QUAL-4)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       HTTPReadTimeout,
		ReadHeaderTimeout: HTTPReadHeaderTimeout,
		WriteTimeout:      HTTPWriteTimeout,
		IdleTimeout:       HTTPIdleTimeout,
		MaxHeaderBytes:    HTTPMaxHeaderBytes,
	}

	// Start HTTP server in background goroutine.
	// Goroutine lifecycle (PHIL-4):
	// - Starts when Start() is called
	// - Exits when server encounters an error (sent to errCh)
	// - Exits when context is canceled and Shutdown() completes
	errCh := make(chan error, 1)
	go func() {
		log.V(1).Info("HTTP server goroutine starting")
		defer log.V(1).Info("HTTP server goroutine exiting")

		var err error
		if c.TLSCertFile != "" && c.TLSKeyFile != "" {
			err = srv.ListenAndServeTLS(c.TLSCertFile, c.TLSKeyFile)
		} else {
			err = srv.ListenAndServe()
		}

		// Server stopped - send error to channel
		// This goroutine will exit after sending
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(err, "HTTP server error")
			return fmt.Errorf("HTTP server failed: %w", err)
		}
		log.Info("HTTP server stopped")
		return nil
	case <-ctx.Done():
		log.Info("Received shutdown signal, gracefully shutting down HTTP server")

		// Use timeout for graceful shutdown (PHIL-5)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), HTTPShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error(err, "Error during graceful shutdown, forcing close")
			// Force close if graceful shutdown fails
			if closeErr := srv.Close(); closeErr != nil {
				log.Error(closeErr, "Error forcing server close")
			}
			return fmt.Errorf("server shutdown error: %w", err)
		}

		log.Info("HTTP server shutdown complete")
		return nil
	}
}
