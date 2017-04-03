package plugin

import (
	"errors"
	"io"

	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

type saveOpts struct {
	plugin string
	output string
}

func newSaveCommand(dockerCli *command.DockerCli) *cobra.Command {
	var opts saveOpts

	cmd := &cobra.Command{
		Use:   "save [OPTIONS] PLUGIN",
		Short: "Save a plugin to a tar archive (streamed to STDOUT by default)",
		Args:  cli.RequiresMinArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// XXX: currently, stick to saving one plugin only.
			opts.plugin = args[0]
			return runSave(dockerCli, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.output, "output", "o", "", "Write to a file, instead of STDOUT")

	return cmd
}

func runSave(dockerCli *command.DockerCli, opts saveOpts) error {
	if opts.output == "" && dockerCli.Out().IsTerminal() {
		return errors.New("Cowardly refusing to save to a terminal. Use the -o flag or redirect.")

	}

	responseBody, err := dockerCli.Client().PluginSave(context.Background(), opts.plugin)
	if err != nil {
		return err
	}
	defer responseBody.Close()

	if opts.output == "" {
		_, err := io.Copy(dockerCli.Out(), responseBody)
		return err
	}

	return command.CopyToFile(opts.output, responseBody)
}
