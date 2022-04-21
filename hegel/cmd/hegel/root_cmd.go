package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tinkerbell/hegel/datamodel"
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
  --use_tls    --grpc-use-tls
  --tls_cert   --grpc-tls-cert
  --tls_key    --grpc-tls-key

Deprecated environment variables
  Deprecated          Current
  DATA_MODEL_VERSION  HEGEL_DATA_MODEL
  HEGEL_TLS_CERT      HEGEL_GRPC_TLS_CERT
  HEGEL_TLS_KEY       HEGEL_GRPC_TLS_KEY
  USE_TLS             HEGEL_GRPC_USE_TLS
  CUSTOM_ENDPOINTS    HEGEL_HTTP_CUSTOM_ENDPOINTS
  TRUSTED_PROXIES     HEGEL_TRUSTED_PROXIES
`

// EnvNamePrefix defines the environment variable prefix required for all environment configuration.
const EnvNamePrefix = "HEGEL"

// RootCommandOptions encompasses all the configurability of the RootCommand.
type RootCommandOptions struct {
	Facility       string `mapstructure:"facility"`
	DataModel      string `mapstructure:"data-model"`
	TrustedProxies string `mapstructure:"trusted-proxies"`

	HTTPPort            int    `mapstructure:"http-port"`
	HTTPCustomEndpoints string `mapstructure:"http-custom-endpoints"`

	GrpcTLSCertPath string `mapstructure:"grpc-tls-cert"`
	GrpcTLSKeyPath  string `mapstructure:"grpc-tls-key"`
	GrpcUseTLS      bool   `mapstructure:"grpc-use-tls"`

	KubeAPI    string `mapstructure:"kubernetes-api"`
	Kubeconfig string `mapstructure:"kubeconfig"`
}

func (o RootCommandOptions) GetDataModel() datamodel.DataModel {
	return datamodel.DataModel(o.DataModel)
}

// RootCommand is the root command that represents the entrypoint to Hegel.
type RootCommand struct {
	*cobra.Command
	Vpr  *viper.Viper
	Opts RootCommandOptions
}

// Temporary workaround to circumvent the linter until the root command is wired up.
var _, _ = NewRootCommand()

// NewRootCommand creates new RootCommand instance.
func NewRootCommand() (*RootCommand, error) {
	rootCmd := &RootCommand{
		Command: &cobra.Command{
			Use:  os.Args[0],
			Long: longHelp,
		},
	}

	rootCmd.PreRunE = rootCmd.PreRun
	rootCmd.RunE = rootCmd.Run
	rootCmd.Flags().SortFlags = false // Print flag help in the order they're specified.

	// Ensure keys with `-` use `_` for env keys else Viper won't match them.
	rootCmd.Vpr = viper.NewWithOptions(viper.EnvKeyReplacer(strings.NewReplacer("-", "_")))
	rootCmd.Vpr.SetEnvPrefix(EnvNamePrefix)

	if err := rootCmd.configureFlags(); err != nil {
		return nil, err
	}

	if err := rootCmd.configureLegacyFlags(); err != nil {
		return nil, err
	}

	return rootCmd, nil
}

// PreRun satisfies cobra.Command.PreRunE and unmarshalls. Its responsible for populating rc.Opts.
func (rc *RootCommand) PreRun(*cobra.Command, []string) error {
	if err := rc.Vpr.Unmarshal(&rc.Opts); err != nil {
		return err
	}

	return rc.validateOpts()
}

// Run executes Hegel. Its temporarily unimplemented.
func (rc *RootCommand) Run(*cobra.Command, []string) error {
	return nil
}

func (rc *RootCommand) configureFlags() error {
	rc.Flags().String("facility", "onprem", "The facility we are running in (mostly to connect to cacher)")
	rc.Flags().String("data-model", string(datamodel.TinkServer), "The back-end data source: [\"1\", \"kubernetes\"] (1 indicates tink server)")
	rc.Flags().String("trusted-proxies", "", "A commma separated list of allowed peer IPs and/or CIDR blocks to replace with X-Forwarded-For for both gRPC and HTTP endpoints")

	rc.Flags().String("grpc-tls-cert", "", "Path of a TLS certificate for the gRPC server")
	rc.Flags().String("grpc-tls-key", "", "Path to the private key for the tls_cert")
	rc.Flags().Bool("grpc-use-tls", true, "Toggle for gRPC TLS usage")

	rc.Flags().Int("http-port", 50061, "Port to listen on for HTTP requests")
	rc.Flags().String("http-custom-endpoints", `{"/metadata":".metadata.instance"}`, "JSON encoded object specifying custom endpoint => metadata mappings")

	rc.Flags().String("kubernetes-api", "", "URL of the Kubernetes API Server")
	rc.Flags().String("kubeconfig", "", "Path to a kubeconfig file")

	if err := rc.Vpr.BindPFlags(rc.Flags()); err != nil {
		return err
	}

	var err error
	rc.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		err = rc.Vpr.BindEnv(f.Name)
	})

	return err
}

func (rc *RootCommand) configureLegacyFlags() error {
	rc.Flags().SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "use_tls":
			return pflag.NormalizedName("grpc-use-tls")
		case "tls_cert":
			return pflag.NormalizedName("grpc-tls-cert")
		case "tls_key":
			return pflag.NormalizedName("grpc-tls-key")
		case "http_port":
			return pflag.NormalizedName("http-port")
		default:
			return pflag.NormalizedName(name)
		}
	})

	for key, envName := range map[string]string{
		"data-model":            "DATA_MODEL_VERSION",
		"grpc-tls-cert":         "HEGEL_TLS_CERT",
		"grpc-tls-key":          "HEGEL_TLS_KEY",
		"grpc-use-tls":          "USE_TLS",
		"http-custom-endpoints": "CUSTOM_ENDPOINTS",
		"trusted-proxies":       "TRUSTED_PROXIES",
	} {
		if err := rc.Vpr.BindEnv(key, envName); err != nil {
			return err
		}
	}

	return nil
}

func (rc *RootCommand) validateOpts() error {
	if rc.Opts.GrpcUseTLS {
		if rc.Opts.GrpcTLSCertPath == "" {
			return errors.New("--grpc-use-tls requires --grpc-tls-cert")
		}

		if rc.Opts.GrpcTLSKeyPath == "" {
			return errors.New("--grpc-use-tls requires --grpc-tls-key")
		}
	}
	if rc.Opts.GetDataModel() == datamodel.Kubernetes {
		if rc.Opts.Kubeconfig == "" {
			return fmt.Errorf("--data-model=%v requires --kubeconfig", datamodel.Kubernetes)
		}
	}

	return nil
}
