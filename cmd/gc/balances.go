package main

import (
	"fmt"
	"github.com/andrewyang17/goBlockchain/node"
	"os"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/spf13/cobra"
)

func balanceCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "balances",
		Short: "Interact with balances (list...).",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return incorrectUsageErr()
		},
		Run: func(cmd *cobra.Command, args []string) {},
	}

	cmd.AddCommand(balancesListCmd())

	return &cmd
}

func balancesListCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "list",
		Short: "Lists all balances.",
		Run: func(cmd *cobra.Command, args []string) {
			state, err := database.NewStateFromDisk(getDataDirFromCmd(cmd), node.DefaultMiningDifficulty)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer state.Close()

			fmt.Printf("Account balances at %x:\n", state.LatestBlockHash())
			fmt.Println("-----------------")
			fmt.Println("")

			for account, balance := range state.Balances {
				fmt.Println(fmt.Sprintf("%s: %d", account.String(), balance))
			}

			fmt.Println("")
			fmt.Println("Accounts nonces:")
			fmt.Println("")
			fmt.Println("-----------------")
			fmt.Println("")

			for account, nonce := range state.Account2Nonce {
				fmt.Println(fmt.Sprintf("%s: %d", account.String(), nonce))
			}
		},
	}

	addDefaultRequiredFlags(&cmd)

	return &cmd
}
