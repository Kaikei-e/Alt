package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var homeFlagsCmd = &cobra.Command{
	Use:   "flags",
	Short: "Show Knowledge Home feature flags",
	Long: `Display current feature flag configuration for Knowledge Home.

Example:
  altctl home flags`,
	RunE: runHomeFlags,
}

func runHomeFlags(cmd *cobra.Command, args []string) error {
	reqBody := map[string]interface{}{}
	var resp struct {
		EnableHomePage      bool `json:"enableHomePage"`
		EnableTracking      bool `json:"enableTracking"`
		EnableProjectionV2  bool `json:"enableProjectionV2"`
		RolloutPercentage   int  `json:"rolloutPercentage"`
		EnableRecallRail    bool `json:"enableRecallRail"`
		EnableLens          bool `json:"enableLens"`
		EnableStreamUpdates bool `json:"enableStreamUpdates"`
		EnableSupersedeUx   bool `json:"enableSupersedeUx"`
	}

	return callAndRenderTable(cmd, "GetFeatureFlags", "Knowledge Home Feature Flags",
		[]string{"FLAG", "VALUE"}, reqBody, &resp, func() [][]string {
			return [][]string{
				{"enable_home_page", fmt.Sprintf("%v", resp.EnableHomePage)},
				{"enable_tracking", fmt.Sprintf("%v", resp.EnableTracking)},
				{"enable_projection_v2", fmt.Sprintf("%v", resp.EnableProjectionV2)},
				{"rollout_percentage", fmt.Sprintf("%d%%", resp.RolloutPercentage)},
				{"enable_recall_rail", fmt.Sprintf("%v", resp.EnableRecallRail)},
				{"enable_lens", fmt.Sprintf("%v", resp.EnableLens)},
				{"enable_stream_updates", fmt.Sprintf("%v", resp.EnableStreamUpdates)},
				{"enable_supersede_ux", fmt.Sprintf("%v", resp.EnableSupersedeUx)},
			}
		})
}

func init() {
	homeCmd.AddCommand(homeFlagsCmd)
	addAdminFlags(homeFlagsCmd)
}
