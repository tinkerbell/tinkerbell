package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/tink/agent"
)

func TestVersionFlag(t *testing.T) {
	c := &config{Options: &agent.Options{}}
	command := &ff.Command{
		Name:  name,
		Usage: "tink-agent [flags]",
		Flags: RegisterAllFlags(c),
	}

	if err := command.Parse([]string{"-version"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !c.PrintVersion {
		t.Fatal("-version did not set PrintVersion")
	}
}

func TestVersionFlagHelp(t *testing.T) {
	c := &config{Options: &agent.Options{}}
	command := &ff.Command{
		Name:  name,
		Usage: "tink-agent [flags]",
		Flags: RegisterAllFlags(c),
	}

	err := command.Parse([]string{"--help"})
	if !errors.Is(err, ff.ErrHelp) {
		t.Fatalf("Parse() error = %v, want %v", err, ff.ErrHelp)
	}
	help := ffhelp.Command(command).String()
	for _, want := range []string{"FLAGS (general)", "-version", "Print the version and exit"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help does not contain %q:\n%s", want, help)
		}
	}
}

func TestContainerdRegistryConfigPathFlag(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"default": {
			want: "/etc/containerd/certs.d",
		},
		"custom path": {
			args: []string{"-containerd-registry-config-path", "/custom/registry-config"},
			want: "/custom/registry-config",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := &config{Options: &agent.Options{}}
			command := &ff.Command{
				Name:  name,
				Usage: "tink-agent [flags]",
				Flags: RegisterAllFlags(c),
			}

			if err := command.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got := c.Options.Runtime.Containerd.RegistryConfigPath; got != tt.want {
				t.Errorf("RegistryConfigPath = %q, want %q", got, tt.want)
			}
		})
	}
}
