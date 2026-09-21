package flag

import (
	"flag"
	"fmt"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffval"
)

// EnvVarPrefix is the prefix ff derives environment variable names from. A flag
// named "dhcp-bind-addr-v6" reads TINKERBELL_DHCP_BIND_ADDR_V6.
const EnvVarPrefix = "TINKERBELL"

// Family is an IP address family. Flags registered with one apply only to that
// family; flags registered without one apply to both.
type Family string

const (
	V4 Family = "v4"
	V6 Family = "v6"
)

// label returns the family as it is written in help text.
func (f Family) label() string {
	if f == V6 {
		return "IPv6"
	}
	return "IPv4"
}

// Config defines the configuration for a flag.
type Config struct {
	Name  string
	Usage string
}

// Set is a wrapper around ff.FlagSet that allows for helper methods to be created.
type Set struct {
	*ff.FlagSet
}

// RegisterFamily registers f as "<name>-<family>", scoped to a single address
// family. One Config therefore serves both families, so a missing counterpart is
// a missing call here rather than an absent declaration.
func (fs *Set) RegisterFamily(f Config, fam Family, fv flag.Value) {
	fs.Register(Config{
		Name:  f.Name + "-" + string(fam),
		Usage: fmt.Sprintf("%s (%s only)", f.Usage, fam.label()),
	}, fv)
}

// Register registers a flag with the provided flag set.
// This will panic if the flag is unable to be added to the flag set, like for a duplicate name.
func (fs *Set) Register(f Config, fv flag.Value) {
	ph := func() string {
		// If the flag is a boolean flag add the static placeholder of "BOOL"
		if _, ok := fv.(*ffval.Bool); ok {
			return "BOOL"
		}
		return ""
	}()

	if _, err := fs.AddFlag(ff.FlagConfig{
		LongName:    f.Name,
		Usage:       f.Usage,
		Value:       fv,
		Placeholder: ph,
	}); err != nil {
		panic(err)
	}
}
