package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage identity",
}

var identityShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		if proj.Identity == nil {
			fmt.Println("No identity configured. Re-run 'dits init' to generate one.")
			return nil
		}

		fmt.Printf("Actor ID:    %s\n", proj.Identity.ActorID)
		fmt.Printf("Public Key:  %s\n", proj.Identity.PublicKey)
		fmt.Printf("Node ID:     %s\n", proj.Config.NodeID)
		return nil
	},
}

func init() {
	identityCmd.AddCommand(identityShowCmd)
}
