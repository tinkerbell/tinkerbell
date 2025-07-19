package migrate

import (
	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/migrate/helm"
)

func NewCommand() *ff.Command {
	return &ff.Command{
		Name:     "migrate",
		Usage:    "migrate [flags]",
		LongHelp: "Migrate Tinkerbell configurations.",
		Flags:    ff.NewFlagSet("migrate"),
		Subcommands: []*ff.Command{
			helm.NewCommand(),
		},
	}
}
