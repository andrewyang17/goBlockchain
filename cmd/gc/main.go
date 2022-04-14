package main

import (
	"fmt"
	"os"

	"github.com/andrewyang17/goBlockchain/fs"
	"github.com/spf13/cobra"
)

const flagKeystoreFile = "keystore"
const flagDataDir = "datadir"
const flagMiner = "miner"
const flagIP = "ip"
const flagPort = "port"
const flagBootstrapAcc = "bootstrap-account"
const flagBootstrapIP = "bootstrap-ip"
const flagBootstrapPort = "bootstrap-port"

func main() {
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Blockchain",
		Run:   func(cmd *cobra.Command, args []string) {},
	}

	cmd.AddCommand(versionCmd)
	cmd.AddCommand(walletCmd())
	cmd.AddCommand(runCmd())
	cmd.AddCommand(balanceCmd())

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addDefaultRequiredFlags(cmd *cobra.Command) {
	cmd.Flags().String(
		flagDataDir,
		"",
		"Absolute path to your node's data dir where the DB will be/is stored")

	cmd.MarkFlagRequired(flagDataDir)
}

func addKeystoreFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagKeystoreFile, "", "Absolute path to the encrypted keystore file")
	cmd.MarkFlagRequired(flagKeystoreFile)
}

func getDataDirFromCmd(cmd *cobra.Command) string {
	dataDir, _ := cmd.Flags().GetString(flagDataDir)

	return fs.ExpandPath(dataDir)
}

func incorrectUsageErr() error {
	return fmt.Errorf("incorrect usage")
}
