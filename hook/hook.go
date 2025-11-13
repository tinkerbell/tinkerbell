package hook

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// Config holds the configuration for the hook service
type Config struct {
	// ImagePath is the directory where hook images are stored
	ImagePath string
	// OCIRegistry is the OCI registry URL (e.g., "ghcr.io")
	OCIRegistry string
	// OCIRepository is the repository path (e.g., "tinkerbell/hook")
	OCIRepository string
	// OCIReference is the image tag or digest (e.g., "latest", "v1.2.3", "sha256:...")
	OCIReference string
	// OCIUsername is the optional username for OCI registry authentication
	OCIUsername string
	// OCIPassword is the optional password for OCI registry authentication
	OCIPassword string
	// PullTimeout for pulling OCI images
	PullTimeout time.Duration
	// HTTPAddr is the address to bind the HTTP server to
	HTTPAddr netip.AddrPort
	// EnableHTTPServer controls whether to start the HTTP file server
	EnableHTTPServer bool
}

// Option functions for configuring the hook service
type Option func(*Config)

func WithImagePath(path string) Option {
	return func(c *Config) {
		c.ImagePath = path
	}
}

func WithOCIRegistry(registry string) Option {
	return func(c *Config) {
		c.OCIRegistry = registry
	}
}

func WithOCIRepository(repository string) Option {
	return func(c *Config) {
		c.OCIRepository = repository
	}
}

func WithOCIReference(reference string) Option {
	return func(c *Config) {
		c.OCIReference = reference
	}
}

func WithPullTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.PullTimeout = timeout
	}
}

func WithOCIUsername(username string) Option {
	return func(c *Config) {
		c.OCIUsername = username
	}
}

func WithOCIPassword(password string) Option {
	return func(c *Config) {
		c.OCIPassword = password
	}
}

func WithHTTPAddr(addr netip.AddrPort) Option {
	return func(c *Config) {
		c.HTTPAddr = addr
	}
}

func WithEnableHTTPServer(enable bool) Option {
	return func(c *Config) {
		c.EnableHTTPServer = enable
	}
}

// NewConfig creates a new hook service configuration with defaults
func NewConfig(opts ...Option) *Config {
	defaults := &Config{
		ImagePath:        "/var/lib/hook",
		OCIRegistry:      "ghcr.io",
		OCIRepository:    "tinkerbell/hook",
		OCIReference:     "latest",
		PullTimeout:      10 * time.Minute,
		EnableHTTPServer: true,
	}

	for _, opt := range opts {
		opt(defaults)
	}

	return defaults
}

// service manages hook image downloads and serving
type service struct {
	config     *Config
	log        logr.Logger
	pullOnce   sync.Once
	mutex      sync.RWMutex
	ready      bool
	httpServer *http.Server
}

// Start initializes and starts the hook service
func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	log.Info("starting hook service",
		"ociRegistry", c.OCIRegistry,
		"ociRepository", c.OCIRepository,
		"ociReference", c.OCIReference,
		"imagePath", c.ImagePath,
		"httpEnabled", c.EnableHTTPServer)

	svc := &service{
		config: c,
		log:    log,
	}

	// Create image directory
	if err := os.MkdirAll(c.ImagePath, 0o755); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	// Check if ImagePath has any files
	if svc.imagePathHasFiles() {
		log.Info("image path contains files, skipping OCI pull")
		svc.ready = true
	} else {
		log.Info("image path is empty, will pull OCI image in background")
	}

	// Start background pull if needed
	go func() {
		if !svc.ready {
			if err := svc.pullOCIImage(ctx); err != nil {
				log.Error(err, "failed to pull OCI image")
			} else {
				svc.mutex.Lock()
				svc.ready = true
				svc.mutex.Unlock()
				log.Info("OCI image pulled and ready")
			}
		}
	}()

	// Start HTTP server if enabled
	if c.EnableHTTPServer && c.HTTPAddr.IsValid() {
		return svc.startHTTPServer(ctx)
	}

	// If HTTP server is not enabled, just wait for context cancellation
	<-ctx.Done()
	return nil
}

// imagePathHasFiles checks if the ImagePath directory contains any files
func (s *service) imagePathHasFiles() bool {
	entries, err := os.ReadDir(s.config.ImagePath)
	if err != nil {
		s.log.Info("unable to read image path", "error", err)
		return false
	}

	// Check if there are any files (not just directories)
	for _, entry := range entries {
		if !entry.IsDir() {
			return true
		}
	}

	return false
}

// pullOCIImage pulls the OCI image from the registry and extracts it to ImagePath
func (s *service) pullOCIImage(ctx context.Context) error {
	var err error
	s.pullOnce.Do(func() {
		err = s.doPullOCIImage(ctx)
		if err != nil {
			s.log.Error(err, "failed to pull OCI image")
		}
	})
	return err
}

// doPullOCIImage performs the actual OCI image pull
func (s *service) doPullOCIImage(ctx context.Context) error {
	log := s.log.WithValues("event", "pull_oci_image",
		"registry", s.config.OCIRegistry,
		"repository", s.config.OCIRepository,
		"reference", s.config.OCIReference,
		"imagePath", s.config.ImagePath)

	log.Info("pulling OCI image")

	// Create a timeout context for the pull operation
	pullCtx, cancel := context.WithTimeout(ctx, s.config.PullTimeout)
	defer cancel()

	// Create a file store for the extracted files
	fileStore, err := file.New(s.config.ImagePath)
	if err != nil {
		return fmt.Errorf("failed to create file store: %w", err)
	}
	defer fileStore.Close()

	// Create a remote repository
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", s.config.OCIRegistry, s.config.OCIRepository))
	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	log.Info("using authenticated registry access")
	authClient := auth.Client{
		Client: &http.Client{
			Timeout: s.config.PullTimeout,
		},
		Cache: auth.NewCache(),
		Credential: auth.StaticCredential(s.config.OCIRegistry, auth.Credential{
			Username: s.config.OCIUsername,
			Password: s.config.OCIPassword,
		}),
	}

	// Configure authentication only if credentials are provided
	if s.config.OCIUsername != "" || s.config.OCIPassword != "" {
		authClient.Credential = auth.StaticCredential(s.config.OCIRegistry, auth.Credential{
			Username: s.config.OCIUsername,
			Password: s.config.OCIPassword,
		})
	}

	repo.Client = &authClient

	// Copy from remote repository to local file store
	reference := s.config.OCIReference
	log.Info("copying OCI image to local file store", "reference", reference)

	desc, err := oras.Copy(pullCtx, repo, reference, fileStore, reference, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("failed to pull OCI image: %w", err)
	}

	log.Info("OCI image pulled successfully",
		"digest", desc.Digest.String(),
		"size", desc.Size,
		"mediaType", desc.MediaType)

	return nil
}

// startHTTPServer starts the HTTP file server
func (s *service) startHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve hook files
	mux.Handle("/", http.FileServerFS(os.DirFS(s.config.ImagePath)))

	s.httpServer = &http.Server{
		Addr:              s.config.HTTPAddr.String(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.log.Info("starting hook HTTP server", "addr", s.config.HTTPAddr.String())

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.log.Error(err, "failed to shutdown HTTP server")
		}
	}()

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}
