package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
	"github.com/alt-project/adrdag/internal/render"
)

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the supersedes DAG (mermaid, byte-compatible with adr_graph.py)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if err := validFormat(format, "mermaid", "dot", "json"); err != nil {
				return err
			}
			adrs, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			g := adr.SupersedesGraph(adrs)

			var body string
			switch format {
			case "mermaid":
				body = render.Mermaid(g, adrs)
			case "dot":
				body = render.DOT(g)
			case "json":
				out, err := render.JSONGraph(adrs, g, graph.BuildReverse(g))
				if err != nil {
					return ioErr(err)
				}
				body = string(out)
			}

			out, _ := cmd.Flags().GetString("out")
			if out == "" || out == "-" {
				fmt.Fprintln(cmd.OutOrStdout(), body)
				return nil
			}
			// parity with adr_graph.py's cmd_graph: write body + "\n", print "wrote <path>"
			if err := os.WriteFile(out, []byte(body+"\n"), 0o644); err != nil {
				return ioErr(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", out)
			return nil
		},
	}
	cmd.Flags().String("format", "mermaid", "output format: mermaid|dot|json")
	cmd.Flags().String("out", "-", "write to file instead of stdout")
	return cmd
}
