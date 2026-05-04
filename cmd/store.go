package cmd

import (
	"fmt"

	"github.com/devenjarvis/lathe/internal/store"
	"github.com/spf13/cobra"
)

var withVerify bool

var storeCmd = &cobra.Command{
	Use:   "store <path>",
	Short: "Save a tutorial directory to ~/.lathe/tutorials/",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tut, err := store.Store(args[0], withVerify)
		if err != nil {
			return err
		}
		fmt.Printf("Stored: %s (%s)\n", tut.Title, tut.Status)
		if withVerify {
			fmt.Println("Verification running in background. Run `lathe serve` to check status.")
		}
		return nil
	},
}

func init() {
	storeCmd.Flags().BoolVar(&withVerify, "verify", false, "spawn background verification after storing")
	rootCmd.AddCommand(storeCmd)
}
