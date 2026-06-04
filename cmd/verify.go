package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/verify"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <slug>",
	Short: "Run verification for a stored tutorial",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\`) {
			return fmt.Errorf("invalid slug: %q", slug)
		}
		tutorialsDir, err := config.TutorialsDir()
		if err != nil {
			return err
		}
		tutDir := filepath.Join(tutorialsDir, slug)

		if err := verify.StartVerification(slug, tutDir); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"Verifying tutorial %q (running in background; refresh `lathe serve` to watch)\n", slug)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
