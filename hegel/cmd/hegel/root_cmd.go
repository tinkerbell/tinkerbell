package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof" //nolint:gosec // G108: Profiling endpoint is automatically exposed on /debug/pprof

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/oklog/run"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tinkerbell/hegel/datamodel"
	"github.com/tinkerbell/hegel/hardware"
	"github.com/tinkerbell/hegel/http"
	"github.com/tinkerbell/hegel/metrics"
)

const longHelp = `
Run a Hegel server.

Each CLI argument has a corresponding environment variable in the form of the CLI argument prefixed with HEGEL. If both
the flag and environment variable form are specified, the flag form takes precedence.

Examples
  --factility              HEGEL_FACILITY
  --http-port              HEGEL_HTTP_PORT
  --http-custom-endpoints  HEGEL_HTTP_CUSTOM_ENDPOINTS

For backwards compatibility a set of deprecated CLI and environment variables are still supported. Behavior for
specifying both deprecated and current forms is undefined.

Deprecated CLI flags
  Deprecated   Current
  --http_port  --http-port

Deprecated environment variables
  Deprecated          Current
  CUSTOM_ENDPOINTS    HEGEL_HTTP_CUSTOM_ENDPOINTS
  DATA_MODEL_VERSION  HEGEL_DATA_MODEL
  TRUSTED_PROXIES     HEGEL_TRUSTED_PROXIES
`

// EnvNamePrefix defines the environment variable prefix required for all environment configuration.
const EnvNamePrefix = "HEGEL"

// RootCommandOptions encompasses all the configurability of the RootCommand.
type RootCommandOptions struct {
	DataModel      string `mapstructure:"data-model"`
	Facility       string `mapstructure:"facility"`
	TrustedProxies string `mapstructure:"trusted-proxies"`

	HTTPCustomEndpoints string `mapstructure:"http-custom-endpoints"`
	HTTPPort            int    `mapstructure:"http-port"`

	KubernetesAPIURL string `mapstructure:"kubernetes"`
	Kubeconfig       string `mapstructure:"kubeconfig"`
	KubeNamespace    string `mapstructure:"kube-namespace"`

	HegelAPI bool `mapstructure:"hegel-api"`
}

func (o RootCommandOptions) GetDataModel() datamodel.DataModel {
	return datamodel.DataModel(o.DataModel)
}

// RootCommand is the root command that represents the entrypoint to Hegel.
type RootCommand struct {
	*cobra.Command
	vpr  *viper.Viper
	Opts RootCommandOptions
}

// NewRootCommand creates new RootCommand instance.
func NewRootCommand() (*RootCommand, error) {
	rootCmd := &RootCommand{
		Command: &cobra.Command{
			Use:          os.Args[0],
			Long:         longHelp,
			SilenceUsage: true,
		},
	}

	rootCmd.PreRunE = rootCmd.PreRun
	rootCmd.RunE = rootCmd.Run
	rootCmd.Flags().SortFlags = false // Print flag help in the order they're specified.

	// Ensure keys with `-` use `_` for env keys else Viper won't match them.
	rootCmd.vpr = viper.NewWithOptions(viper.EnvKeyReplacer(strings.NewReplacer("-", "_")))
	rootCmd.vpr.SetEnvPrefix(EnvNamePrefix)

	if err := rootCmd.configureFlags(); err != nil {
		return nil, err
	}

	if err := rootCmd.configureLegacyFlags(); err != nil {
		return nil, err
	}

	return rootCmd, nil
}

// PreRun satisfies cobra.Command.PreRunE and unmarshalls. Its responsible for populating c.Opts.
func (c *RootCommand) PreRun(*cobra.Command, []string) error {
	return c.vpr.Unmarshal(&c.Opts)
}

// Run executes Hegel.
func (c *RootCommand) Run(cmd *cobra.Command, _ []string) error {
	logger, err := log.Init("github.com/tinkerbell/hegel")
	if err != nil {
		return errors.Errorf("initialize logger: %v", err)
	}
	defer logger.Close()

	logger.Package("main").With("opts", fmt.Sprintf("%+v", c.Opts)).Info("root command options")

	ctx, otelShutdown := otelinit.InitOpenTelemetry(cmd.Context(), "hegel")
	defer otelShutdown(ctx)

	metrics.State.Set(metrics.Initializing)

	hardwareClient, err := hardware.NewClient(hardware.ClientConfig{
		Model:         c.Opts.GetDataModel(),
		Facility:      c.Opts.Facility,
		KubeAPI:       c.Opts.KubernetesAPIURL,
		Kubeconfig:    c.Opts.Kubeconfig,
		KubeNamespace: c.Opts.KubeNamespace,
	})
	if err != nil {
		return errors.Errorf("create client: %v", err)
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	var routines run.Group

	routines.Add(
		func() error {
			return http.Serve(
				ctx,
				logger,
				hardwareClient,
				c.Opts.HTTPPort,
				time.Now(),
				c.Opts.GetDataModel(),
				c.Opts.HTTPCustomEndpoints,
				c.Opts.TrustedProxies,
				c.Opts.HegelAPI,
			)
		},
		func(error) { cancel() },
	)

	return routines.Run()
}

func (c *RootCommand) configureFlags() error {
	// Alphabetically ordereed
	c.Flags().String("data-model", string(datamodel.TinkServer), "The back-end data source: [\"1\", \"kubernetes\"] (1 indicates tink server)")
	c.Flags().String("facility", "onprem", "The facility we are running in (mostly to connect to cacher)")

	c.Flags().String("http-custom-endpoints", `{"/metadata":".metadata.instance"}`, "JSON encoded object specifying custom endpoint => metadata mappings")
	c.Flags().Int("http-port", 50061, "Port to listen on for HTTP requests")

	c.Flags().String("kubeconfig", "", "Path to a kubeconfig file")
	c.Flags().String("kubernetes", "", "URL of the Kubernetes API Server")
	c.Flags().String("kube-namespace", "", "The Kubernetes namespace to target; defaults to the service account")

	c.Flags().String("trusted-proxies", "", "A commma separated list of allowed peer IPs and/or CIDR blocks to replace with X-Forwarded-For")

	c.Flags().Bool("hegel-api", false, "Toggle to true to enable Hegel's new experimental API. Default is false.")

	if err := c.vpr.BindPFlags(c.Flags()); err != nil {
		return err
	}

	var err error
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		err = c.vpr.BindEnv(f.Name)
	})

	return err
}

func (c *RootCommand) configureLegacyFlags() error {
	c.Flags().SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "http_port":
			return pflag.NormalizedName("http-port")
		default:
			return pflag.NormalizedName(name)
		}
	})

	for key, envName := range map[string]string{
		"data-model":            "DATA_MODEL_VERSION",
		"http-custom-endpoints": "CUSTOM_ENDPOINTS",
		"trusted-proxies":       "TRUSTED_PROXIES",
	} {
		if err := c.vpr.BindEnv(key, envName); err != nil {
			return err
		}
	}

	return nil
}
