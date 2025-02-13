package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	retry "github.com/avast/retry-go"
	"github.com/aws/smithy-go/ptr"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/pkg/conv"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

type Config struct {
	Log          logr.Logger
	Client       *client.Client
	RegistryAuth *registry.AuthConfig
}

func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	pullImage := func() error {
		pullOpts := image.PullOptions{}

		if c.RegistryAuth != nil {
			encodedJSON, err := json.Marshal(c.RegistryAuth)
			if err != nil {
				return fmt.Errorf("unable to encode auth config: %w", err)
			}
			pullOpts.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
		}

		img, err := c.Client.ImagePull(ctx, a.Image, pullOpts)
		if err != nil {
			// If the image is already present, we can ignore the error.
			// This might be the case where the image is already present in the local cache
			// and the environment doesn't have access to the registry.
			// Embedded images in HookOS are a partial example of this.
			if _, _, err := c.Client.ImageInspectWithRaw(ctx, a.Image); err == nil {
				return nil
			}
			return fmt.Errorf("docker: %w", err)
		}
		defer img.Close()

		// Docker requires everything to be read from the images ReadCloser for the image to actually
		// be pulled. We may want to log image pulls in a circular buffer somewhere for debug-ability.
		if _, err = io.Copy(io.Discard, img); err != nil {
			return fmt.Errorf("docker: %w", err)
		}

		return nil
	}

	err := retry.Do(pullImage, retry.Attempts(5), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return err
	}

	// TODO: Support all the other things on the action such as volumes.
	cfg := container.Config{
		Image: a.Image,
		Env:   conv.ParseEnv(a.Env),
	}

	hostCfg := container.HostConfig{
		Binds:      []string{},
		Privileged: true,
	}
	if a.Namespaces.PID != "" {
		hostCfg.PidMode = container.PidMode(a.Namespaces.PID)
	}
	for _, v := range a.Volumes {
		hostCfg.Binds = append(hostCfg.Binds, string(v))
	}

	containerName := conv.ParseName(a.ID, a.Name)

	// Docker uses the entrypoint as the default command. The Tink Action Cmd property is modeled
	// as being the command launched in the container hence it is used as the entrypoint. Args
	// on the action are therefore the command portion in Docker.
	if a.Cmd != "" {
		cfg.Entrypoint = append(cfg.Entrypoint, a.Cmd)
	}
	if len(a.Args) > 0 {
		cfg.Cmd = append(cfg.Cmd, a.Args...)
	}

	// TODO: Figure out container logging. We probably want to save it somewhere for debug-ability.

	create, err := c.Client.ContainerCreate(ctx, &cfg, &hostCfg, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}

	// Always try to remove the container on exit.
	defer func() {
		// Force remove containers in an attempt to preserve space in memory constraints environments.
		// In rare cases this may create orphaned volumes that the Docker CLI won't clean up.
		opts := container.RemoveOptions{Force: true}

		// We can't use the context passed to Run() as it may have been cancelled so we use Background()
		// instead.
		// give the context 10 seconds to remove the container
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := c.Client.ContainerRemove(ctx, create.ID, opts)
		if err != nil {
			c.Log.Info("Couldn't remove container", "container_name", containerName, "error", err)
		}
	}()

	// Issue the wait with a 'next-exit' condition so we can await a response originating from
	// ContainerStart().
	waitBody, waitErr := c.Client.ContainerWait(ctx, create.ID, container.WaitConditionNextExit)

	if err := c.Client.ContainerStart(ctx, create.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}

	select {
	case result := <-waitBody:
		if result.StatusCode == 0 {
			return nil
		}
		return fmt.Errorf("got non 0 exit status, see the logs for more information")

	case err := <-waitErr:
		return fmt.Errorf("error while waiting for container: %w", err)

	case <-ctx.Done():
		// We can't use the context passed to Run() as its been cancelled.
		err := c.Client.ContainerStop(context.Background(), create.ID, container.StopOptions{
			Timeout: ptr.Int(5),
		})
		if err != nil {
			c.Log.Info("Failed to gracefully stop container", "error", err)
		}
		return fmt.Errorf("context error: %w", ctx.Err())
	}
}
