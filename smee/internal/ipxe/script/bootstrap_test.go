package script

import (
	"net"
	"strings"
	"testing"
)

func renderHook(t *testing.T, bootstrapURL string) string {
	t.Helper()
	got, err := GenerateTemplate(Hook{
		Arch:         "x86_64",
		DownloadURL:  "http://10.0.0.1:8080/osie",
		Retries:      2,
		RetryDelay:   1,
		BootstrapURL: bootstrapURL,
	}, HookScript)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestHookScriptBootstrapOrdering pins the constraint that makes the delivery
// work: the kernel applies initramfs archives in order and later ones win, so
// the bootstrap archive must be fetched after the OS initramfs.
func TestHookScriptBootstrapOrdering(t *testing.T) {
	const bootstrapURL = "http://10.0.0.1:8080/bootstrap/de:ed:be:ef:fe:ed/bootstrap.cpio"
	got := renderHook(t, bootstrapURL)

	osie := strings.Index(got, "initrd ${download-url}/${initrd}")
	boot := strings.Index(got, "initrd "+bootstrapURL)
	switch {
	case osie == -1:
		t.Fatal("script does not load the OS initramfs")
	case boot == -1:
		t.Fatal("script does not load the bootstrap archive")
	case boot < osie:
		t.Fatal("bootstrap archive is loaded before the OS initramfs; it would be overwritten")
	}
}

// TestHookScriptBootstrapHasItsOwnFailurePath guards against a transient fetch
// failure silently booting a machine with no configuration.
func TestHookScriptBootstrapHasItsOwnFailurePath(t *testing.T) {
	got := renderHook(t, "http://10.0.0.1:8080/bootstrap/de:ed:be:ef:fe:ed/bootstrap.cpio")

	for _, want := range []string{
		":retry_bootstrap",
		"goto bootstrap-error",
		":bootstrap-error",
		"echo Failed to load bootstrap data",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script is missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "initrd http://10.0.0.1:8080/bootstrap/de:ed:be:ef:fe:ed/bootstrap.cpio && goto boot ||") {
		t.Error("bootstrap fetch does not fall through to boot on success")
	}
}

// TestHookScriptWithoutBootstrapIsUnchanged keeps the feature invisible to
// deployments that do not enable it.
func TestHookScriptWithoutBootstrapIsUnchanged(t *testing.T) {
	got := renderHook(t, "")

	for _, unwanted := range []string{
		"download_bootstrap",
		"retry_bootstrap",
		"bootstrap-error",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("script mentions %q when bootstrap delivery is disabled:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "initrd ${download-url}/${initrd} && goto boot ||") {
		t.Errorf("initrd line should jump straight to boot when bootstrap is disabled:\n%s", got)
	}
}

func TestBootstrapURL(t *testing.T) {
	mac, err := net.ParseMAC("de:ed:be:ef:fe:ed")
	if err != nil {
		t.Fatal(err)
	}
	if got := bootstrapURL("", mac); got != "" {
		t.Errorf("disabled: got %q, want \"\"", got)
	}
	// Root-relative: a bare relative path would resolve against the script's directory.
	if got, want := bootstrapURL("/bootstrap/", mac), "/bootstrap/de:ed:be:ef:fe:ed/bootstrap.cpio"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
