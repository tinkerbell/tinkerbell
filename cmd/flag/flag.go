package flag

import (
	"flag"

	"github.com/peterbourgon/ff/v4"
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
	if _, err := fs.AddFlag(ff.FlagConfig{
		LongName: f.Name,
		Usage:    f.Usage,
		Value:    fv,
	}); err != nil {
		panic(err)
	}
}
