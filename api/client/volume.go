package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"text/tabwriter"
	"text/template"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/opts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/parsers/filters"
	"github.com/docker/docker/pkg/units"
)

func (cli *DockerCli) CmdVolume(args ...string) error {
	description := "Manage Docker Volumes\n\nCommands:\n"
	commands := [][]string{
		{"ls", "List volumes"},
		{"inspect", "Inspect a volume"},
		{"create", "Create a volume"},
		{"rm", "Remove a volume"},
	}

	for _, cmd := range commands {
		description += fmt.Sprintf("    %-10.10s%s\n", cmd[0], cmd[1])
	}

	description += "\nRun 'docker volume COMMNAD --help' for more information on a command."

	cmd := cli.Subcmd("volume", nil, description, true)
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		cmd.Usage()
		return nil
	}

	return cli.CmdVolumeLs(args...)
}

func (cli *DockerCli) CmdVolumeLs(args ...string) error {
	cmd := cli.Subcmd("volume ls", nil, "List volumes", true)

	quiet := cmd.Bool([]string{"q", "-quiet"}, false, "Only display volume names")
	size := cmd.Bool([]string{"s", "-size"}, false, "Display total size of volumes")

	flFilter := opts.NewListOpts(nil)
	cmd.Var(&flFilter, []string{"f", "-filter"}, "Provide filter values (i.e. 'dangling=true')")

	cmd.Require(flag.Exact, 0)
	cmd.ParseFlags(args, true)

	volFilterArgs := filters.Args{}
	for _, f := range flFilter.GetAll() {
		var err error
		volFilterArgs, err = filters.ParseFlag(f, volFilterArgs)
		if err != nil {
			return err
		}
	}

	v := url.Values{}
	if *size {
		v.Set("size", "1")
	}

	if len(volFilterArgs) > 0 {
		filterJson, err := filters.ToParam(volFilterArgs)
		if err != nil {
			return err
		}
		v.Set("filters", filterJson)
	}

	rdr, _, _, err := cli.call("GET", "/volumes?"+v.Encode(), nil, nil)
	if err != nil {
		return err
	}

	var volumes types.VolumesListResponse
	if err := json.NewDecoder(rdr).Decode(&volumes); err != nil {
		return err
	}

	w := tabwriter.NewWriter(cli.out, 20, 1, 3, ' ', 0)
	if !*quiet {
		fmt.Fprintf(w, "VOLUME NAME\tDRIVER")
		if *size {
			fmt.Fprintf(w, "\tSIZE")
		}
		fmt.Fprintf(w, "\n")
	}

	for _, vol := range volumes.Volumes {
		if *quiet {
			fmt.Fprintf(w, "%s\n", vol.Name)
			continue
		}

		fmt.Fprintf(w, "%s\t%s", vol.Name, vol.Driver)
		if *size {
			humanSize := units.HumanSize(float64(vol.Size))
			fmt.Fprintf(w, "\t%s", humanSize)
		}
		fmt.Fprintf(w, "\n")
	}
	w.Flush()
	return nil
}

func (cli *DockerCli) CmdVolumeInspect(args ...string) error {
	cmd := cli.Subcmd("volume inspect", []string{"[DRIVER NAME] [VOLUME NAME]"}, "Inspect a volume", true)
	tmplStr := cmd.String([]string{"f", "-format"}, "", "Format the output using the given go template.")
	size := cmd.Bool([]string{"s", "-size"}, false, "Show the size of the volume in output")
	if err := cmd.Parse(args); err != nil {
		return nil
	}

	cmd.Require(flag.Exact, 2)
	cmd.ParseFlags(args, true)

	var tmpl *template.Template
	if *tmplStr != "" {
		var err error
		tmpl, err = template.New("").Funcs(funcMap).Parse(*tmplStr)
		if err != nil {
			return err
		}
	}

	indented := new(bytes.Buffer)

	driver := cmd.Args()[0]
	name := cmd.Args()[1]
	v := url.Values{}
	if *size {
		v.Set("size", "1")
	}
	obj, _, err := readBody(cli.call("GET", "/volumes/"+driver+"/"+name+"?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}

	if tmpl == nil {
		if err := json.Indent(indented, obj, "", "    "); err != nil {
			return err
		}
	} else {
		rdr := bytes.NewReader(obj)
		dec := json.NewDecoder(rdr)

		var volume *types.Volume

		if err := json.NewDecoder(rdr).Decode(&volume); err != nil {
			return err
		}

		if err := tmpl.Execute(cli.out, volume); err != nil {
			rdr.Seek(0, 0)
			var raw interface{}
			if err := dec.Decode(&raw); err != nil {
				return err
			}
			if err := tmpl.Execute(cli.out, raw); err != nil {
				return err
			}
		}
		cli.out.Write([]byte{'\n'})
	}

	if tmpl == nil {
		if _, err := io.Copy(cli.out, indented); err != nil {
			return err
		}
	}

	return nil
}

func (cli *DockerCli) CmdVolumeCreate(args ...string) error {
	cmd := cli.Subcmd("volume create", nil, "Create a volume", true)
	flDriver := cmd.String([]string{"d", "-driver"}, "local", "Specify volume driver name")
	flName := cmd.String([]string{"-name"}, "", "Sepcify volume name")

	flDriverOpts := opts.NewMapOpt(nil)
	cmd.Var(flDriverOpts, []string{"o", "-opt"}, "Set driver specific options")

	cmd.Require(flag.Exact, 0)
	cmd.ParseFlags(args, true)

	volReq := &types.VolumeCreateRequest{
		Driver:     *flDriver,
		DriverOpts: flDriverOpts.GetAll(),
	}

	if *flName != "" {
		volReq.Name = *flName
	}

	rdr, _, _, err := cli.call("POST", "/volumes", volReq, nil)
	if err != nil {
		return err
	}

	var vol types.Volume
	if err := json.NewDecoder(rdr).Decode(&vol); err != nil {
		return err
	}
	fmt.Fprintf(cli.out, "%s\n", vol.Name)
	return nil
}

func (cli *DockerCli) CmdVolumeRm(args ...string) error {
	cmd := cli.Subcmd("volume rm", nil, "Remove a volume", true)
	cmd.Require(flag.Exact, 2)
	cmd.ParseFlags(args, true)

	driver := args[0]
	name := args[1]

	_, _, _, err := cli.call("DELETE", "/volumes/"+driver+"/"+name, nil, nil)
	return err
}
