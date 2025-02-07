package flag

import (
	"flag"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffval"
)

// Config defines the configuration for a flag.
type Config struct {
	Name  string
	Usage string
}

// Set is a wrapper around ff.FlagSet that allows for helper methods to be created.
type Set struct {
	*ff.FlagSet
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
