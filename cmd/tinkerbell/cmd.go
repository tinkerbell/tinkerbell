package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/spf13/pflag"
	"github.com/tinkerbell/tinkerbell/apiserver"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/config/crd"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/rufio"
	"github.com/tinkerbell/tinkerbell/secondstar"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/controller"
	"github.com/tinkerbell/tinkerbell/tink/server"
	"github.com/tinkerbell/tinkerbell/tootles"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func Execute(ctx context.Context, cancel context.CancelFunc, args []string) error {
	globals := &flag.GlobalConfig{
		BackendKubeConfig: func() string {
			hd, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			p := filepath.Join(hd, ".kube", "config")
			// if this default location doesn't exist it's highly
			// likely that Tinkerbell is being run from within the
			// cluster. In that case, the loading of the Kubernetes
			// client will only look for in cluster configuration/environment
			// variables if this is empty.
			_, oserr := os.Stat(p)
			if oserr != nil {
				return ""
			}
			return p
		}(),
		PublicIP:             detectPublicIPv4(),
		EnableSmee:           true,
		EnableTootles:        true,
		EnableTinkServer:     true,
		EnableTinkController: true,
		EnableRufio:          true,
		EnableSecondStar:     true,
		EmbeddedGlobalConfig: flag.EmbeddedGlobalConfig{
			EnableKubeAPIServer: true,
			EnableETCD:          true,
		},
	}

	s := &flag.SmeeConfig{
		Config: smee.NewConfig(smee.Config{}, detectPublicIPv4()),
	}

	h := &flag.TootlesConfig{
		Config:   tootles.NewConfig(tootles.Config{}, fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 50061)),
		BindAddr: detectPublicIPv4(),
		BindPort: 50061,
	}
	ts := &flag.TinkServerConfig{
		Config:   server.NewConfig(),
		BindAddr: detectPublicIPv4(),
		BindPort: 42113,
	}
	controllerOpts := []controller.Option{
		controller.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8080))),
		controller.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8081))),
		controller.WithEnableLeaderElection(false),
	}
	tc := &flag.TinkControllerConfig{
		Config: controller.NewConfig(controllerOpts...),
	}

	rufioOpts := []rufio.Option{
		rufio.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8082))),
		rufio.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8083))),
		rufio.WithBmcConnectTimeout(2 * time.Minute),
		rufio.WithPowerCheckInterval(30 * time.Minute),
		rufio.WithEnableLeaderElection(false),
	}
	rc := &flag.RufioConfig{
		Config: rufio.NewConfig(rufioOpts...),
	}

	ssc := &flag.SecondStarConfig{
		Config: &secondstar.Config{
			SSHPort:      2222,
			IPMITOOLPath: "/usr/sbin/ipmitool",
			IdleTimeout:  15 * time.Minute,
		},
	}

	ecfg := embed.NewConfig()
	ecfg.Dir = "/tmp/default.etcd"
	ec := &flag.EmbeddedEtcdConfig{
		Config:             ecfg,
		WaitHealthyTimeout: time.Minute,
	}

	// order here determines the help output.
	kaffs := ff.NewFlagSet("embedded kube-apiserver")
	efs := ff.NewFlagSet("embedded etcd").SetParent(kaffs)
	sfs := ff.NewFlagSet("smee - DHCP and iPXE service").SetParent(efs)
	hfs := ff.NewFlagSet("tootles - Metadata service").SetParent(sfs)
	tfs := ff.NewFlagSet("tink server - Workflow service").SetParent(hfs)
	cfs := ff.NewFlagSet("tink controller - Workflow controller").SetParent(tfs)
	rfs := ff.NewFlagSet("rufio - BMC controller").SetParent(cfs)
	ssfs := ff.NewFlagSet("secondstar - SSH over serial service").SetParent(rfs)
	gfs := ff.NewFlagSet("globals").SetParent(ssfs)
	flag.RegisterSmeeFlags(&flag.Set{FlagSet: sfs}, s)
	flag.RegisterTootlesFlags(&flag.Set{FlagSet: hfs}, h)
	flag.RegisterTinkServerFlags(&flag.Set{FlagSet: tfs}, ts)
	flag.RegisterTinkControllerFlags(&flag.Set{FlagSet: cfs}, tc)
	flag.RegisterRufioFlags(&flag.Set{FlagSet: rfs}, rc)
	flag.RegisterSecondStarFlags(&flag.Set{FlagSet: ssfs}, ssc)
	flag.RegisterEtcd(&flag.Set{FlagSet: efs}, ec)
	flag.RegisterGlobal(&flag.Set{FlagSet: gfs}, globals)

	apiserverFS, runFunc := apiserver.ConfigAndFlags(ctx)

	apiserverFS.VisitAll(func(f *pflag.Flag) {
		// help and v already exist in the global flags defined above so we skip them
		// here to avoid duplicate flag errors.
		if f.Name == "help" || f.Name == "v" {
			return
		}
		fc := ff.FlagConfig{
			LongName: f.Name,
			Usage:    f.Usage,
			Value:    f.Value,
		}
		// feature-gates has a lot of output and includes a lot of '\n' characters
		// that makes the ffhelp output not output all the flags. We remove all the
		// feature gates so that all the flags are output in the help.
		if f.Name == "feature-gates" {
			lines := strings.Split(f.Usage, "\n")
			newlines := make([]string, 0)
			for _, line := range lines {
				if !strings.HasPrefix(line, "kube:") {
					newlines = append(newlines, line)
				}
			}
			fc.Usage = strings.Join(newlines, "\n")
		}

		if len([]rune(f.Shorthand)) > 0 {
			fc.ShortName = []rune(f.Shorthand)[0]
		}

		if _, err := kaffs.AddFlag(fc); err != nil {
			fmt.Printf("error adding flag: %v\n", err)
		}
	})
	cli := &ff.Command{
		Name:     "tinkerbell",
		Usage:    "tinkerbell [flags]",
		LongHelp: "Tinkerbell stack.",
		Flags:    gfs,
	}

	if err := cli.Parse(args, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		e := errors.New(ffhelp.Command(cli).String())
		if !errors.Is(err, ff.ErrHelp) {
			e = fmt.Errorf("%w\n%s", e, err)
		}

		return e
	}

	// Smee
	s.Convert(&globals.TrustedProxies, globals.PublicIP)

	// Tootles
	h.Convert(&globals.TrustedProxies)

	// Tink Server
	ts.Convert()

	// Tink Controller
	if !inCluster() && tc.Config.EnableLeaderElection && tc.Config.LeaderElectionNamespace == "" {
		tc.Config.LeaderElectionNamespace = "default"
	}

	// Rufio
	if !inCluster() && rc.Config.EnableLeaderElection && rc.Config.LeaderElectionNamespace == "" {
		rc.Config.LeaderElectionNamespace = "default"
	}

	// Second star
	if err := ssc.Convert(); err != nil {
		return fmt.Errorf("failed to convert secondstar config: %w", err)
	}

	log := defaultLogger(globals.LogLevel)
	log.Info("starting tinkerbell",
		"version", gitRevision(),
		"smeeEnabled", globals.EnableSmee,
		"tootlesEnabled", globals.EnableTootles,
		"tinkServerEnabled", globals.EnableTinkServer,
		"tinkControllerEnabled", globals.EnableTinkController,
		"rufioEnabled", globals.EnableRufio,
		"secondStarEnabled", globals.EnableSecondStar,
		"publicIP", globals.PublicIP,
		"embeddedKubeAPIServer", globals.EmbeddedGlobalConfig.EnableKubeAPIServer,
		"embeddedEtcd", globals.EmbeddedGlobalConfig.EnableETCD,
	)

	// Etcd server
	if globals.EmbeddedGlobalConfig.EnableETCD {
		ec.Config.ZapLoggerBuilder = embed.NewZapLoggerBuilder(zapLogger(globals.LogLevel))
		e, err := embed.StartEtcd(ec.Config)
		if err != nil {
			return fmt.Errorf("failed to start etcd: %w", err)
		}
		defer e.Close()
		select {
		case <-e.Server.ReadyNotify():
			log.Info("etcd server is ready")
		case <-time.After(ec.WaitHealthyTimeout):
			e.Server.Stop() // trigger a shutdown
			return fmt.Errorf("server took too long to start")
		}
		go func() {
			<-ctx.Done()
			e.Server.Stop()
		}()
	} else {
		log.Info("embedded etcd is disabled")
	}

	// API Server
	g := errgroup.Group{}
	g.Go(func() error {
		if globals.EmbeddedGlobalConfig.EnableKubeAPIServer {
			if err := runFunc(apiserverFS, log.WithValues("service", "kube-apiserver")); err != nil {
				return fmt.Errorf("API server error: %w", err)
			}
			return nil
		}
		log.Info("embedded kube-apiserver is disabled")
		return nil
	})

	if numEnabled(globals) == 0 {
		goto WAIT
	}
	switch globals.Backend {
	case "kube":
		if err := crdMigrations(ctx, log, globals.BackendKubeConfig, globals.BackendKubeNamespace); err != nil {
			cancel()
			gerr := g.Wait()
			return fmt.Errorf("CRD migrations failed: %w", errors.Join(err, gerr))
		}

		// I think we need some time for the CRDs to be "available" before we can create a client with indexers that use them.
		// TODO: validate this assumption.
		<-time.After(3 * time.Second)

		b, err := newKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace, enabledIndexes(globals.EnableSmee, globals.EnableTootles, globals.EnableTinkServer, globals.EnableSecondStar))
		if err != nil {
			return fmt.Errorf("failed to create kube backend: %w", err)
		}
		s.Config.Backend = b
		h.Config.BackendEc2 = b
		h.Config.BackendHack = b
		ts.Config.Backend = b
		tc.Config.Client = b.ClientConfig
		rc.Config.Client = b.ClientConfig
		ssc.Config.Backend = b
	case "file":
		b, err := newFileBackend(ctx, log, globals.BackendFilePath)
		if err != nil {
			return fmt.Errorf("failed to create file backend: %w", err)
		}
		s.Config.Backend = b
	case "none":
		b := newNoopBackend()
		s.Config.Backend = b
		h.Config.BackendEc2 = b
		h.Config.BackendHack = b
	default:
		return fmt.Errorf("unknown backend %q", globals.Backend)
	}

WAIT:

	// Smee
	g.Go(func() error {
		if globals.EnableSmee {
			if err := s.Config.Start(ctx, log.WithValues("service", "smee")); err != nil {
				return fmt.Errorf("failed to start smee service: %w", err)
			}
			return nil
		}
		log.Info("smee service is disabled")
		return nil
	})

	// Tootles
	g.Go(func() error {
		if globals.EnableTootles {
			if err := h.Config.Start(ctx, log.WithValues("service", "tootles")); err != nil {
				return fmt.Errorf("failed to start tootles service: %w", err)
			}
			return nil
		}
		log.Info("tootles service is disabled")
		return nil
	})

	// Tink Server
	g.Go(func() error {
		if globals.EnableTinkServer {
			if err := ts.Config.Start(ctx, log.WithValues("service", "tink-server")); err != nil {
				return fmt.Errorf("failed to start tink server service: %w", err)
			}
			return nil
		}
		log.Info("tink server service is disabled")
		return nil
	})

	// Tink Controller
	g.Go(func() error {
		if globals.EnableTinkController {
			if err := tc.Config.Start(ctx, log.WithValues("service", "tink-controller")); err != nil {
				return fmt.Errorf("failed to start tink controller service: %w", err)
			}
			return nil
		}
		log.Info("tink controller service is disabled")
		return nil
	})

	// Rufio
	g.Go(func() error {
		if globals.EnableRufio {
			if err := rc.Config.Start(ctx, log.WithValues("service", "rufio")); err != nil {
				return fmt.Errorf("failed to start rufio service: %w", err)
			}
			return nil
		}
		log.Info("rufio service is disabled")
		return nil
	})

	// SecondStar - SSH over serial
	g.Go(func() error {
		if globals.EnableSecondStar {
			if err := ssc.Config.Start(ctx, log.WithValues("service", "secondstar")); err != nil {
				return fmt.Errorf("failed to start secondstar service: %w", err)
			}
			return nil
		}
		log.Info("secondstar service is disabled")
		return nil
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	if numEnabled(globals) == 0 && !globals.EmbeddedGlobalConfig.EnableKubeAPIServer && !globals.EmbeddedGlobalConfig.EnableETCD {
		return errors.New("all services are disabled")
	}

	// if etcd is the only service enabled, wait until the context is done.
	if numEnabled(globals) == 0 && !globals.EmbeddedGlobalConfig.EnableKubeAPIServer && globals.EmbeddedGlobalConfig.EnableETCD {
		<-ctx.Done()
	}

	return nil
}

func numEnabled(globals *flag.GlobalConfig) int {
	n := 0
	if globals.EnableSmee {
		n++
	}
	if globals.EnableTootles {
		n++
	}
	if globals.EnableTinkServer {
		n++
	}
	if globals.EnableTinkController {
		n++
	}
	if globals.EnableRufio {
		n++
	}
	if globals.EnableSecondStar {
		n++
	}
	return n
}

func crdMigrations(ctx context.Context, log logr.Logger, kubeconfig, namespace string) error {
	backendNoIndexes, err := newKubeBackend(ctx, kubeconfig, "", namespace, nil)
	if err != nil {
		return fmt.Errorf("failed to create kube backend: %w", err)
	}

	// Wait for the API server to be healthy
	if err := waitForAPIServer(ctx, log, backendNoIndexes.ClientConfig, 20*time.Second, 5*time.Second); err != nil {
		return fmt.Errorf("failed to wait for API server health: %w", err)
	}

	apiExtClient, err := apiextensionsv1client.NewForConfig(backendNoIndexes.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	for _, raw := range [][]byte{crd.HardwareCRD, crd.TemplateCRD, crd.WorkflowCRD, crd.MachineCRD, crd.JobCRD, crd.TaskCRD} {
		obj := &unstructured.Unstructured{}
		if _, _, err := decoder.Decode(raw, nil, obj); err != nil {
			return fmt.Errorf("failed to decode YAML: %w", err)
		}
		crdef := &apiextensionsv1.CustomResourceDefinition{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, crdef); err != nil {
			return fmt.Errorf("failed to convert unstructured to CRD: %w", err)
		}

		if _, err := apiExtClient.CustomResourceDefinitions().Create(ctx, crdef, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CRD: %w", err)
		}
	}
	return nil
}

// waitForAPIServer waits for the Kubernetes API server to become healthy.
func waitForAPIServer(ctx context.Context, log logr.Logger, config *rest.Config, maxWaitTime time.Duration, pollInterval time.Duration) error {
	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Calculate deadline
	deadline := time.Now().Add(maxWaitTime)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Check health
			resp, err := clientset.Discovery().RESTClient().Get().AbsPath("/livez").Do(ctx).Raw()

			if err == nil && len(resp) > 0 {
				return nil // API server is healthy
			}

			// Log error and continue waiting
			log.Info("API server not healthy yet", "reason", err)

			// Sleep before next check
			time.Sleep(pollInterval)
		}
	}

	return fmt.Errorf("timed out waiting for API server health after %v", maxWaitTime)
}

func enabledIndexes(smeeEnabled, tootlesEnabled, tinkServerEnabled, secondStarEnabled bool) map[kube.IndexType]kube.Index {
	idxs := make(map[kube.IndexType]kube.Index, 0)

	if smeeEnabled {
		idxs = flag.KubeIndexesSmee
	}
	if tootlesEnabled {
		for k, v := range flag.KubeIndexesTootles {
			idxs[k] = v
		}
	}
	if tinkServerEnabled {
		for k, v := range flag.KubeIndexesTinkServer {
			idxs[k] = v
		}
	}
	if secondStarEnabled {
		for k, v := range flag.KubeIndexesSecondStar {
			idxs[k] = v
		}
	}

	return idxs
}

// defaultLogger uses the slog logr implementation.
func defaultLogger(level int) logr.Logger {
	// source file and function can be long. This makes the logs less readable.
	// for improved readability, truncate source file to last 3 parts and remove the function entirely.
	customAttr := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			ss, ok := a.Value.Any().(*slog.Source)
			if !ok || ss == nil {
				return a
			}

			p := strings.Split(ss.File, "/")
			// log the file path from tinkerbell/tinkerbell to the end.
			var idx int

			for i, v := range p {
				if v == "tinkerbell" {
					if i+2 < len(p) {
						idx = i + 2
						break
					}
				}
				// This trims the source file for 3rd party packages to include
				// just enough information to identify the package. Without this,
				// the source file can be long and make the log line more cluttered
				// and hard to read.
				if v == "mod" {
					if i+1 < len(p) {
						idx = i + 1
						break
					}
				}
			}
			ss.File = filepath.Join(p[idx:]...)
			ss.File = fmt.Sprintf("%s:%d", ss.File, ss.Line)
			a.Value = slog.StringValue(ss.File)
			a.Key = "caller"

			return a
		}

		// This changes the slog.Level string representation to an integer.
		// This makes it so that the V-levels passed in to the CLI show up as is in the logs.
		if a.Key == slog.LevelKey {
			b, ok := a.Value.Any().(slog.Level)
			if !ok {
				return a
			}
			a.Value = slog.Float64Value(math.Abs(float64(b)))
			return a
		}

		return a
	}
	opts := &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.Level(-level),
		ReplaceAttr: customAttr,
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	return logr.FromSlogHandler(log.Handler())
}

// zapLogger is used by embedded etcd. It's the only logger that embedded etcd supports.
func zapLogger(level int) *zap.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(-level))
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(fmt.Sprintf("%d", l))
	}
	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	return logger.With(zap.String("service", "etcd"))
}

// inCluster checks if we are running in cluster.
func inCluster() bool {
	if _, err := rest.InClusterConfig(); err == nil {
		return true
	}
	return false
}
