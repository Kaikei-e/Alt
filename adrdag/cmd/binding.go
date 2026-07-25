package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
)

func newBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binding",
		Short: "List every currently-binding ADR (accepted AND not superseded)",
		Long:  "Lists the corpus's current set of latest-effective decisions:\nbinding(A) ⇔ status=accepted ∧ no inbound supersedes edge.\nAn empty result is a success — an all-non-binding corpus is a `check`\nfailure (status drift / cycle), not a binding-command failure.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if err := validFormat(format, "text", "json"); err != nil {
				return err
			}
			adrs, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			reverse := graph.BuildReverse(adr.SupersedesGraph(adrs))

			type bindingADR struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}
			bindings := []bindingADR{}
			for _, id := range adr.SortedIDs(adrs) {
				if graph.IsBinding(adrs[id].Status, id, reverse) {
					bindings = append(bindings, bindingADR{ID: id, Title: adrs[id].Title})
				}
			}

			if format == "json" {
				out, err := json.MarshalIndent(bindings, "", "  ")
				if err != nil {
					return ioErr(err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			for _, b := range bindings {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", b.ID, b.Title)
			}
			return nil
		},
	}
	cmd.Flags().String("format", "text", "output format: text|json")
	return cmd
}
