package main

import (
	"context"
	"fmt"
	"os"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/andrewyang17/goBlockchain/node"
	"github.com/spf13/cobra"
)

func runCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "run",
		Short: "Launches the Blockchain node and its HTTP API.",
		Run: func(cmd *cobra.Command, args []string) {
			miner, _ := cmd.Flags().GetString(flagMiner)
			ip, _ := cmd.Flags().GetString(flagIP)
			port, _ := cmd.Flags().GetUint64(flagPort)

			bootstrapIP, _ := cmd.Flags().GetString(flagBootstrapIP)
			bootstrapPort, _ := cmd.Flags().GetUint64(flagBootstrapPort)
			bootstrapAcc, _ := cmd.Flags().GetString(flagBootstrapAcc)

			fmt.Println("Launching Blockchain node and its HTTP API...")

			bootstrap := node.NewPeerNode(
				bootstrapIP,
				bootstrapPort,
				true,
				database.NewAccount(bootstrapAcc),
				false,
			)

			n := node.New(getDataDirFromCmd(cmd), ip, port, database.NewAccount(miner), bootstrap, node.DefaultMiningDifficulty)

			if err := n.Run(context.Background()); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	addDefaultRequiredFlags(&cmd)
	cmd.Flags().String(flagMiner, node.DefaultMiner, "miner account of this node to receive block rewards")
	cmd.Flags().String(flagIP, node.DefaultIP, "exposed IP for communication with peers")
	cmd.Flags().Uint64(flagPort, node.DefaultHttpPort, "exposed HTTP port for communication with peers")

	cmd.Flags().String(flagBootstrapIP, node.DefaultBootstrapIP, "default bootstrap server to interconnect peers")
	cmd.Flags().Uint64(flagBootstrapPort, node.DefaultBootstrapPort, "default bootstrap server port to interconnect peers")
	cmd.Flags().String(flagBootstrapAcc, node.DefaultBootstrapAcc, "default bootstrap account to interconnect peers")

	return &cmd
}
