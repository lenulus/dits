package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lenulus/pf/internal/project"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new DITS project",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectKey, _ := cmd.Flags().GetString("project")
		if projectKey == "" {
			return fmt.Errorf("--project is required")
		}
		projectKey = strings.ToUpper(projectKey)

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		proj, err := project.Init(dir, projectKey)
		if err != nil {
			return fmt.Errorf("initializing project: %w", err)
		}
		defer proj.DB.Close()

		fmt.Printf("Initialized DITS project '%s' in %s\n", projectKey, dir)
		return nil
	},
}

func init() {
	initCmd.Flags().StringP("project", "p", "", "Project key (e.g., MYPROJ)")
	initCmd.MarkFlagRequired("project")
}
