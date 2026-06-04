package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/store"
	"github.com/devenjarvis/lathe/internal/verify"
	"github.com/spf13/cobra"
)

var withVerify bool

var storeCmd = &cobra.Command{
	Use:   "store <path>",
	Short: "Save a tutorial directory to ~/.lathe/tutorials/",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tut, err := store.Store(args[0])
		if err != nil {
			return err
		}
		if withVerify {
			tutorialsDir, err := config.TutorialsDir()
			if err != nil {
				return err
			}
			tutDir := filepath.Join(tutorialsDir, tut.Slug)
			if err := verify.StartVerification(tut.Slug, tutDir); err != nil {
				return fmt.Errorf("start verification: %w", err)
			}
			tut.Status = store.StatusVerifying
		}
		fmt.Printf("Stored: %s (%s)\n", tut.Title, tut.Status)
		if withVerify {
			fmt.Println("Verification running in background. Run `lathe serve` to check status.")
		}
		return nil
	},
}

func init() {
	storeCmd.Flags().BoolVar(&withVerify, "verify", false, "run verification after storing")
	rootCmd.AddCommand(storeCmd)
}
