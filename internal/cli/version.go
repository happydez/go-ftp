package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/happydez/go-ftp/internal/version"
)

func newVersionCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of go-ftp",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd.OutOrStdout(), output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text, short or json")

	return cmd
}

func printVersion(w io.Writer, output string) error {
	info := version.Get()
	switch output {
	case "text":
		_, err := fmt.Fprintf(w, "go-ftp %s (commit %s, built %s, %s, %s)\n",
			info.Version, info.Commit, info.Date, info.GoVersion, info.Platform)
		return err
	case "short":
		_, err := fmt.Fprintln(w, info.Version)
		return err
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	default:
		return newUsageError("unknown output format %q, want text, short or json", output)
	}
}
