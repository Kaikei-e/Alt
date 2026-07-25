package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
)

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <adr-id>",
		Short: "Resolve an ADR id to its currently-effective successor(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if err := validFormat(format, "text", "json"); err != nil {
				return err
			}
			adrs, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			id := adr.NormalizeID(args[0])
			if _, ok := adrs[id]; !ok {
				fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: unknown ADR %s\n", id)
				return &cliError{code: exitFailure}
			}
			reverse := graph.BuildReverse(adr.SupersedesGraph(adrs))
			effective := graph.Resolve(id, reverse)

			if format == "json" {
				out, err := json.MarshalIndent(struct {
					ID        string   `json:"id"`
					Effective []string `json:"effective"`
				}{ID: id, Effective: effective}, "", "  ")
				if err != nil {
					return ioErr(err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			for _, leaf := range effective {
				fmt.Fprintln(cmd.OutOrStdout(), leaf)
			}
			return nil
		},
	}
	cmd.Flags().String("format", "text", "output format: text|json")
	return cmd
}
