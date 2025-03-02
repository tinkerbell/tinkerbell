//go:build embedded
// +build embedded

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccoveille/go-safecast"
	"github.com/peterbourgon/ff/v4"
	"github.com/spf13/pflag"
	"github.com/tinkerbell/tinkerbell/apiserver"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	// defaults for etcd
	ecfg := embed.NewConfig()
	ecfg.Dir = "/tmp/default.etcd"
	ec := &flag.EmbeddedEtcdConfig{
		Config:             ecfg,
		WaitHealthyTimeout: time.Minute,
	}
	// register flags
	kaffs := ff.NewFlagSet("embedded kube-apiserver")
	efs := ff.NewFlagSet("embedded etcd").SetParent(kaffs)
	flag.RegisterEtcd(&flag.Set{FlagSet: efs}, ec)
	apiserverFS, runFunc := apiserver.ConfigAndFlags()
	apiserverFS.VisitAll(kubeAPIServerFlags(kaffs))
	embeddedFlagSet = efs

	// register the run command
	embeddedApiserverExecute = runFunc
	embeddedKubeControllerManagerExecute = apiserver.Kubecontrollermanager
	embeddedEtcdExecute = func(ctx context.Context, logLevel int) error {
		log := zapLogger(logLevel)
		ec.Config.ZapLoggerBuilder = embed.NewZapLoggerBuilder(log)
		e, err := embed.StartEtcd(ec.Config)
		if err != nil {
			return fmt.Errorf("failed to start etcd: %w", err)
		}
		defer e.Close()
		select {
		case <-e.Server.ReadyNotify():
			log.Info("etcd server is ready")
			// readyChan <- struct{}{}
		case <-time.After(ec.WaitHealthyTimeout):
			// apiserverShutdown.Wait()
			e.Server.Stop() // trigger a shutdown
			return fmt.Errorf("server took too long to become healthy")
		case <-ctx.Done():
			// apiserverShutdown.Wait()
			e.Server.Stop() // trigger a shutdown
			log.Info("context cancelled waiting for etcd to become healthy")
			return nil
		}
		<-ctx.Done()
		// need to wait for the kube apiserver to shutdown before stopping etcd.
		//apiserverShutdown.Wait()
		e.Server.Stop()
		return nil
	}
}

func kubeAPIServerFlags(kaffs *ff.FlagSet) func(*pflag.Flag) {
	return func(f *pflag.Flag) {
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
	}
}

// zapLogger is used by embedded etcd. It's the only logger that embedded etcd supports.
func zapLogger(level int) *zap.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	l, err := safecast.ToInt8(level)
	if err != nil {
		l = 0
	}
	config.Level = zap.NewAtomicLevelAt(zapcore.Level(-l))
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
