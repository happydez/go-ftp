package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/happydez/go-ftp/internal/ftpx"
	"github.com/happydez/go-ftp/internal/transfer"
	"github.com/happydez/go-ftp/internal/ui"
)

const timeLayout = "2006-01-02 15:04"

func newLsCmd(g *globalOptions) *cobra.Command {
	var (
		output    string
		recursive bool
	)

	cmd := &cobra.Command{
		Use:     "ls [PATH]",
		Aliases: []string{"list"},
		Short:   "List a directory on the server",
		Long: "ls shows what is on the server under a path, which is taken relative to\n" +
			"the base_dir of the active profile. With no path it lists base_dir itself.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			s, err := g.session()
			if err != nil {
				return err
			}

			client, err := s.client()
			if err != nil {
				return err
			}
			defer client.Close()

			remote, err := s.remotePath(path)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			entry, found, err := client.Stat(ctx, remote)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("%s is not on the server", remote)
			}

			entries, err := readEntries(ctx, client, entry, remote, recursive)
			if err != nil {
				return err
			}

			return writeEntries(cmd.OutOrStdout(), entries, remote, output)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&output, "output", "o", "table", "output format: table, wide, json or name")
	f.BoolVarP(&recursive, "recursive", "R", false, "descend into every directory under the path")

	return cmd
}

// readEntries is the listing itself. A path that names a file lists just that
// file, the way ls does.
func readEntries(ctx context.Context, client *ftpx.Client, entry ftpx.Entry, remote string, recursive bool) ([]ftpx.Entry, error) {
	if !entry.Dir {
		return []ftpx.Entry{entry}, nil
	}

	if recursive {
		return client.Walk(ctx, remote)
	}

	return client.List(ctx, remote)
}

func writeEntries(w io.Writer, entries []ftpx.Entry, remote, output string) error {
	switch output {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if entries == nil {
			entries = []ftpx.Entry{}
		}
		return encoder.Encode(entries)
	case "name":
		for _, item := range entries {
			if _, err := fmt.Fprintln(w, displayName(item, remote)); err != nil {
				return err
			}
		}
		return nil
	case "table":
		return writeTable(w, entries, remote, false)
	case "wide":
		return writeTable(w, entries, remote, true)
	default:
		return newUsageError("unknown output format %q, want table, wide, json or name", output)
	}
}

// writeTable sends everything to w, the footer included.
func writeTable(w io.Writer, entries []ftpx.Entry, remote string, wide bool) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintf(w, "%s is empty\n", remote)
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	header := "NAME\tSIZE\tMODIFIED"
	if wide {
		header = "TYPE\tNAME\tSIZE\tMODIFIED\tPATH"
	}
	if _, err := fmt.Fprintln(tw, header); err != nil {
		return err
	}

	var (
		files int
		bytes int64
	)

	for _, item := range entries {
		if !item.Dir {
			files++
			bytes += item.Size
		}

		columns := []string{displayName(item, remote), sizeColumn(item), modified(item)}
		if wide {
			columns = append([]string{kindOf(item)}, append(columns, item.Path)...)
		}

		if _, err := fmt.Fprintln(tw, strings.Join(columns, "\t")); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := fmt.Fprintf(w, "\n%d file(s), %s\n", files, ui.Bytes(bytes))

	return err
}

func displayName(entry ftpx.Entry, remote string) string {
	if name := transfer.Relative(remote, entry.Path); name != "" {
		return name
	}
	return entry.Name
}

func sizeColumn(entry ftpx.Entry) string {
	if entry.Dir {
		return "-"
	}
	return ui.Bytes(entry.Size)
}

func modified(entry ftpx.Entry) string {
	if entry.ModTime.IsZero() {
		return "-"
	}
	return entry.ModTime.Local().Format(timeLayout)
}

func kindOf(entry ftpx.Entry) string {
	switch {
	case entry.Dir:
		return "dir"
	case entry.Link:
		return "link"
	default:
		return "file"
	}
}
