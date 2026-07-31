package workflow

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"sigs.k8s.io/yaml"
)

// maxRenderBytes caps rendered template output to guard against expansion-based DoS.
const maxRenderBytes = 256 * 1024

// templateFuncs defines the custom functions available to workflow templates.
var templateFuncs = map[string]interface{}{
	"formatPartition":       formatPartition,
	"netmaskToPrefixLength": netmaskToPrefixLength,
	"toYaml":                toYaml,
	"fromYaml":              fromYaml,
}

// safeFuncMap returns the functions available to workflow templates. It uses
// Sprig's hermetic function map, which excludes non-repeatable and unsafe
// functions such as env, expandenv, and getHostByName.
func safeFuncMap() template.FuncMap {
	fm := sprig.HermeticTxtFuncMap()
	for k, v := range templateFuncs {
		fm[k] = v
	}
	return fm
}

// renderTemplate parses and executes a Go template with the hermetic function
// map, erroring on missing keys and capping output at maxRenderBytes.
func renderTemplate(name, tmplStr string, data interface{}) ([]byte, error) {
	t, err := template.New(name).
		Option("missingkey=error").
		Funcs(safeFuncMap()).
		Parse(tmplStr)
	if err != nil {
		return nil, err
	}

	w := &limitedWriter{limit: maxRenderBytes}
	if err := t.Execute(w, data); err != nil {
		return nil, err
	}

	return w.buf.Bytes(), nil
}

// limitedWriter buffers written bytes and errors once limit is exceeded.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.limit {
		return 0, fmt.Errorf("rendered template exceeds %d bytes", w.limit)
	}
	return w.buf.Write(p)
}

// formatPartition formats a device path with partition for the device type.
// It will never return just the dev.
// if dev has prefix "/dev/disk/", then partitions are always suffixed "-partX" no matter the device type.
// otherwise, if dev ends in a digit, then partitions are suffixed with "pX" (e.g. /dev/nvme0n1 -> /dev/nvme0n1p1).
// otherwise, partitions are suffixed with "X" (e.g. /dev/sda -> /dev/sda1).
func formatPartition(dev string, partition int) string {
	if strings.HasPrefix(dev, "/dev/disk/") {
		return fmt.Sprintf("%v-part%v", dev, partition)
	}
	if len(dev) > 0 && dev[len(dev)-1] >= '0' && dev[len(dev)-1] <= '9' {
		return fmt.Sprintf("%vp%v", dev, partition)
	}
	return fmt.Sprintf("%v%v", dev, partition)
}

// netmaskToPrefixLength converts a netmask (e.g. 255.255.255.0) to prefix length (e.g. 24).
// Returns an error if the netmask is invalid or not IPv4.
func netmaskToPrefixLength(netmask string) (string, error) {
	// Parse the netmask
	ip := net.ParseIP(netmask)
	if ip == nil {
		return "", fmt.Errorf("invalid netmask format: %s", netmask)
	}

	// Convert to IPv4
	ipv4 := ip.To4()
	if ipv4 == nil {
		return "", fmt.Errorf("netmask must be IPv4: %s", netmask)
	}

	// Count the number of 1 bits in the netmask
	ones, _ := net.IPMask(ipv4).Size()
	return strconv.Itoa(ones), nil
}

// toYaml marshals a value to a YAML string.
// Returns an error if marshalling fails, which halts template execution.
func toYaml(v interface{}) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toYaml: %w", err)
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

// fromYaml unmarshals a YAML string into a Go value (map, slice, or scalar).
// Returns an error if the input is empty or unmarshalling fails, which halts template execution.
func fromYaml(str string) (interface{}, error) {
	if str == "" {
		return nil, fmt.Errorf("fromYaml: empty YAML input")
	}
	var out interface{}
	if err := yaml.Unmarshal([]byte(str), &out); err != nil {
		return nil, fmt.Errorf("fromYaml: %w", err)
	}
	return out, nil
}
