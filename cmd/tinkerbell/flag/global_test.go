package flag

import (
	"net/netip"
	"testing"

	"github.com/peterbourgon/ff/v4"
)

func TestRegisterGlobalBindAddressEnv(t *testing.T) {
	t.Setenv("TINKERBELL_BIND_ADDRESS_V4", "192.0.2.10")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix(EnvVarPrefix)); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.BindAddr, netip.MustParseAddr("192.0.2.10"); got != want {
		t.Errorf("BindAddr = %v, want %v", got, want)
	}
}

func TestRegisterGlobalReferenceRules(t *testing.T) {
	const allow = `{"reference":{"resource":["configmaps"]}}`
	const deny = `{"reference":{"resource":["secrets"]}}`
	t.Setenv("TINKERBELL_BACKEND_KUBE_HARDWARE_REFERENCE_DENY_LIST_RULES", deny)

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}
	if err := cmd.Parse([]string{"--backend-kube-hardware-reference-allow-list-rules=" + allow}, ff.WithEnvVarPrefix(EnvVarPrefix)); err != nil {
		t.Fatal(err)
	}
	if got := cfg.BackendKubeOptions.HardwareReferenceAllowListRules; len(got) != 1 || got[0] != allow {
		t.Errorf("allow rules = %v, want %s", got, allow)
	}
	if got := cfg.BackendKubeOptions.HardwareReferenceDenyListRules; len(got) != 1 || got[0] != deny {
		t.Errorf("deny rules = %v, want %s", got, deny)
	}
}

// RegisterFamily hands each address flag the family its name declares, so a
// value from the wrong family is rejected at parse time rather than producing a
// listener for the other family.
func TestRegisterGlobalRejectsWrongFamily(t *testing.T) {
	for name, tt := range map[string]struct{ env, value string }{
		"IPv6 in the v4 flag": {"TINKERBELL_BIND_ADDRESS_V4", "::"},
		"IPv4 in the v6 flag": {"TINKERBELL_BIND_ADDRESS_V6", "0.0.0.0"},
		"IPv4 in public v6":   {"TINKERBELL_PUBLIC_IP_V6", "192.0.2.10"},
		"4-in-6 in public v6": {"TINKERBELL_PUBLIC_IP_V6", "::ffff:192.0.2.10"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)

			fs := ff.NewFlagSet("test")
			RegisterGlobal(&Set{FlagSet: fs}, &GlobalConfig{})
			cmd := &ff.Command{Name: "test", Flags: fs}

			if err := cmd.Parse(nil, ff.WithEnvVarPrefix(EnvVarPrefix)); err == nil {
				t.Fatalf("%s=%s: got nil error, want a family mismatch", tt.env, tt.value)
			}
		})
	}
}

func TestRegisterGlobalPublicIPv6Env(t *testing.T) {
	t.Setenv("TINKERBELL_PUBLIC_IP_V6", "2001:db8::15")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix(EnvVarPrefix)); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.PublicIPv6, netip.MustParseAddr("2001:db8::15"); got != want {
		t.Errorf("PublicIPv6 = %v, want %v", got, want)
	}
}
