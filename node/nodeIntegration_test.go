package node

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/andrewyang17/goBlockchain/fs"
	"github.com/andrewyang17/goBlockchain/wallet"
	"github.com/ethereum/go-ethereum/common"
)

const testKsSpongebobAccount = "0x3eb92807f1f91a8d4d85bc908c7f86dcddb1df57"
const testKsPatrickAccount = "0x6fdc0d8d15ae6b4ebf45c52fd2aafbcbb19a65c8"
const testKsSpongebobFile = "test_spongebob--3eb92807f1f91a8d4d85bc908c7f86dcddb1df57"
const testKsPatrickFile = "test_patrick--6fdc0d8d15ae6b4ebf45c52fd2aafbcbb19a65c8"
const testKsAccountsPwd = "security123"

func TestNode_Run(t *testing.T) {
	dataDir, err := getTestDataDirPath()
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(dataDir)

	n := New(dataDir, "127.0.0.1", 8085, database.NewAccount(DefaultMiner), PeerNode{}, defaultTestMiningDifficulty)

	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)

	err = n.Run(ctx)
	if !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}

func TestNode_Mining(t *testing.T) {
	dataDir, spongebob, patrick, err := setupTestNodeDir(1000000, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(dataDir)

	nInfo := NewPeerNode(
		"127.0.0.1",
		8085,
		false,
		patrick,
		true,
	)

	n := New(dataDir, nInfo.IP, nInfo.Port, spongebob, nInfo, defaultTestMiningDifficulty)

	ctx, closeNode := context.WithTimeout(context.Background(), 20*time.Minute)

	// Schedule a new TX in 5 seconds from now.
	go func() {
		time.Sleep(5 * time.Second)

		tx := database.NewBaseTx(spongebob, patrick, 1, 1, "")
		signedTx, err := wallet.SignTxWithKeystoreAccount(tx, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}

		err = n.AddPendingTX(signedTx, nInfo)
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}
	}()

	// Schedule a TX with insufficient funds in 7 seconds,
	// the AddPendingTX won't add it to the Mempool.
	go func() {
		time.Sleep(7 * time.Second)

		tx := database.NewBaseTx(patrick, spongebob, 50, 1, "")
		signedTx, err := wallet.SignTxWithKeystoreAccount(tx, patrick, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}

		err = n.AddPendingTX(signedTx, nInfo)
		t.Log(err)
		if err == nil {
			t.Errorf("TX should not be added to Mempool because Patrick doesn't have %d GoCoin", tx.Value)
			closeNode()
			return
		}
	}()

	// Schedule a new TX in 12 seconds from now simulating
	// that it came in while the first TX is being mined.
	go func() {
		time.Sleep(2 + miningIntervalSeconds*time.Second)

		tx := database.NewBaseTx(spongebob, patrick, 2, 2, "")
		signedTx, err := wallet.SignTxWithKeystoreAccount(tx, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}

		err = n.AddPendingTX(signedTx, nInfo)
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}
	}()

	// Periodically check if we mined the 2 blocks every 10 seconds.
	go func() {
		ticker := time.NewTicker(10 * time.Second)

		for {
			select {
			case <-ticker.C:
				if n.state.LatestBlock().Header.Number == 2 {
					closeNode()
					return
				}
			}
		}
	}()

	_ = n.Run(ctx)

	if n.state.LatestBlock().Header.Number != 2 {
		t.Fatal("2 pending TX not mined into 2 blocks under 20m")
	}
}

func TestNode_ForgedTx(t *testing.T) {
	dataDir, spongebob, patrick, err := setupTestNodeDir(1000000, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(dataDir)

	nInfo := NewPeerNode(
		"127.0.0.1",
		8085,
		false,
		spongebob,
		true,
	)

	n := New(dataDir, nInfo.IP, nInfo.Port, spongebob, nInfo, defaultTestMiningDifficulty)
	ctx, closeNode := context.WithTimeout(context.Background(), 20*time.Minute)

	txValue := uint(5)
	txNonce := uint(1)
	tx := database.NewBaseTx(spongebob, patrick, txValue, txNonce, "")

	signedTx, err := wallet.SignTxWithKeystoreAccount(tx, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
	if err != nil {
		closeNode()
		t.Fatal(err)
	}

	go func() {
		// Wait for the node to run.
		time.Sleep(1 * time.Second)

		err = n.AddPendingTX(signedTx, nInfo)
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		var wasForgedTxAdded bool

		for {
			select {
			case <-ticker.C:
				if !n.state.LatestBlockHash().IsEmpty() {
					if wasForgedTxAdded && !n.isMining {
						closeNode()
						return
					}
				}

				if !wasForgedTxAdded {
					// Attempt to forge the same TX but with modified time,
					// because the TX.time changed, the TX.signature will be considered forged.
					forgedTx := database.NewBaseTx(spongebob, patrick, txValue, txNonce, "")

					// Use the signature from a valid TX
					forgedSignedTx := database.NewSignedTx(forgedTx, signedTx.Sig)

					err = n.AddPendingTX(forgedSignedTx, nInfo)
					t.Log(err)
					if err == nil {
						t.Error("adding a forged TX to the mempool should not be possible")
						closeNode()
						return
					}
				}

				wasForgedTxAdded = true
			}
		}
	}()

	_ = n.Run(ctx)

	if n.state.LatestBlock().Header.Number != 1 {
		t.Fatal("was suppose to mine only one TX. The second TX was forged")
	}

	if n.state.Balances[patrick] != txValue {
		t.Fatal("forged TX succeeded")
	}
}

func TestNode_ReplayedTx(t *testing.T) {
	dataDir, spongebob, patrick, err := setupTestNodeDir(1000000, 0)
	if err != nil {
		t.Error(err)
	}
	defer fs.RemoveDir(dataDir)

	n := New(dataDir, "127.0.0.1", 8085, spongebob, PeerNode{}, defaultTestMiningDifficulty)

	ctx, closeNode := context.WithCancel(context.Background())
	spongebobPeerNode := NewPeerNode("127.0.0.1", 8085, false, spongebob, true)
	patrickPeerNode := NewPeerNode("127.0.0.1", 8086, false, patrick, true)

	txValue := uint(5)
	txNonce := uint(1)
	tx := database.NewBaseTx(spongebob, patrick, txValue, txNonce, "")

	signedTx, err := wallet.SignTxWithKeystoreAccount(tx, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
	if err != nil {
		t.Error(err)
		closeNode()
		return
	}

	go func() {
		// Wait for the node to run.
		time.Sleep(1 * time.Second)

		err = n.AddPendingTX(signedTx, spongebobPeerNode)
		if err != nil {
			t.Error(err)
			closeNode()
			return
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		var wasReplayedTxAdded bool

		for {
			select {
			case <-ticker.C:
				if !n.state.LatestBlockHash().IsEmpty() {
					if wasReplayedTxAdded && !n.isMining {
						closeNode()
						return
					}
				}

				// Spongebob's original TX got mined.
				// Execute the attack by replaying the TX again.
				if !wasReplayedTxAdded {
					// Simulate the TX was submitted to different node.
					n.archivedTXs = make(map[string]database.SignedTx)

					// Execute the attack.
					err = n.AddPendingTX(signedTx, patrickPeerNode)
					t.Log(err)
					if err == nil {
						t.Error("re-adding a TX to the mempool should not be possible because of Nonce.")
						closeNode()
						return
					}
				}

				wasReplayedTxAdded = true
			}
		}
	}()

	_ = n.Run(ctx)

	if n.state.Balances[patrick] == txValue*2 {
		t.Errorf("replayed attack was successful.")
		return
	}

	if n.state.Balances[patrick] != txValue {
		t.Errorf("replayed attack was successful.")
		return
	}

	if n.state.LatestBlock().Header.Number == 2 {
		t.Errorf("the second block was not suppose to be persisted because it's a replayed TX.")
		return
	}
}

// The test logic summary:
//	- Patrick runs the node
//  - Patrick tries to mine 2 TXs
//  	- The mining gets interrupted because a new block from Spongebob gets synced
//		- Spongebob will get the block reward for this synced block
//		- The synced block contains 1 of the TXs Patrick tried to mine
//	- Patrick tries to mine 1 TX left
//		- Patrick succeeds and gets her block reward
func TestNode_MiningStopsOnNewSyncedBlock(t *testing.T) {
	tc := []struct {
		name     string
		ForkTIP1 uint64
	}{
		{"Legacy", 35},  // Prior ForkTIP1 was activated on number 35.
		{"ForkTIP1", 0}, // To test new blocks when the ForkTIP1 is active.
	}

	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			spongebob := database.NewAccount(testKsSpongebobAccount)
			patrick := database.NewAccount(testKsPatrickAccount)

			dataDir, err := getTestDataDirPath()
			if err != nil {
				t.Fatal(err)
			}

			genesisBalances := make(map[common.Address]uint)
			genesisBalances[spongebob] = 1000000
			genesis := database.Genesis{Balances: genesisBalances, ForkTIP1: tc.ForkTIP1}
			genesisJson, err := json.Marshal(genesis)
			if err != nil {
				t.Fatal(err)
			}

			err = database.InitDataDirIfNotExists(dataDir, genesisJson)
			defer fs.RemoveDir(dataDir)

			err = copyKeystoreFilesIntoTestDataDirPath(dataDir)
			if err != nil {
				t.Fatal(err)
			}

			nInfo := NewPeerNode(
				"127.0.0.1",
				8085,
				false,
				database.NewAccount(""),
				true,
			)

			n := New(dataDir, nInfo.IP, nInfo.Port, patrick, nInfo, uint(3))

			ctx, closeNode := context.WithTimeout(context.Background(), 20*time.Minute)

			tx1 := database.NewBaseTx(spongebob, patrick, 1, 1, "")
			tx2 := database.NewBaseTx(spongebob, patrick, 2, 2, "")

			if tc.name == "Legacy" {
				tx1.Gas = 0
				tx1.GasPrice = 0
				tx2.Gas = 0
				tx2.GasPrice = 0
			}

			signedTx1, err := wallet.SignTxWithKeystoreAccount(tx1, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
			if err != nil {
				t.Error(err)
				return
			}

			signedTx2, err := wallet.SignTxWithKeystoreAccount(tx2, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
			if err != nil {
				t.Error(err)
				return
			}

			tx2Hash, err := signedTx2.Hash()
			if err != nil {
				t.Error(err)
				return
			}

			// Pre-mine a valid block without running the `n.Run()`,
			// with Spongebob as a miner who will receive the block reward,
			// to simulate the block came on the fly from another peer.
			validPreMinedPb := NewPendingBlock(database.Hash{}, 0, spongebob, []database.SignedTx{signedTx1})
			validSyncedBlock, err := Mine(ctx, validPreMinedPb, defaultTestMiningDifficulty)
			if err != nil {
				t.Fatal(err)
			}

			// Add 2 new TXs into the Patrick's node, triggers mining
			go func() {
				time.Sleep(8 * time.Second)

				err := n.AddPendingTX(signedTx1, nInfo)
				if err != nil {
					t.Fatal(err)
				}

				err = n.AddPendingTX(signedTx2, nInfo)
				if err != nil {
					t.Fatal(err)
				}
			}()

			// Interrupt the previously started mining with a new synced block.
			// BUT this block contains only 1 TX the previous mining activity tried to mine,
			// which means the mining will start again for the one pending TX that is left and wasn't in
			// the synced block.
			go func() {
				time.Sleep(2 + miningIntervalSeconds*time.Second)
				if !n.isMining {
					t.Fatal("should be mining")
				}

				// Change the mining difficulty back to the testing level from previously purposefully slow, high value
				// otherwise the synced block would be invalid.
				n.ChangeMiningDifficulty(defaultTestMiningDifficulty)
				_, err := n.state.AddBlock(validSyncedBlock)
				if err != nil {
					t.Fatal(err)
				}
				// Mock the Spongebob's block came from a network
				n.newSyncedBlocks <- validSyncedBlock

				time.Sleep(time.Second)
				if n.isMining {
					t.Fatal("synced block should have canceled mining")
				}

				// Mined TX1 by Spongebob should be removed from the Mempool
				_, onlyTX2IsPending := n.pendingTXs[tx2Hash.Hex()]

				if len(n.pendingTXs) != 1 && !onlyTX2IsPending {
					t.Fatal("synced block should have canceled mining of already mined TX")
				}
			}()

			go func() {
				// Regularly check whenever both TXs are now mined
				ticker := time.NewTicker(10 * time.Second)

				for {
					select {
					case <-ticker.C:
						if n.state.LatestBlock().Header.Number == 2 {
							closeNode()
							return
						}
					}
				}
			}()

			go func() {
				time.Sleep(2 * time.Second)

				// Take a snapshot of the DB balances
				// before the mining is finished and the 2 blocks
				// are created.
				startingSpongebobBalance := n.state.Balances[spongebob]
				startingPatrickBalance := n.state.Balances[patrick]

				// Wait until the 30 mins timeout is reached or
				// the 2 blocks got already mined and the closeNode() was triggered.
				<-ctx.Done()

				endSpongebobBalance := n.state.Balances[spongebob]
				endPatrickBalance := n.state.Balances[patrick]

				// In TX1 Spongebob transferred 1 GoCoin to Patrick.
				// In TX2 Spongebob transferred 2 GoCoins to Patrick.

				var expectedEndSpongebobBalance uint
				var expectedEndPatrickBalance uint

				// Spongebob will occur the cost of SENDING 2 TXs but will collect the reward for mining one block with tx1 in it.
				// Patrick will RECEIVE value from 2 TXs and will also collect the reward for mining one block with tx2 in it.

				if n.state.IsTIP1Fork() {
					expectedEndSpongebobBalance = startingSpongebobBalance - tx1.Cost(true) - tx2.Cost(true) + database.BlockReward + tx1.GasCost()
					expectedEndPatrickBalance = startingPatrickBalance + tx1.Value + tx2.Value + database.BlockReward + tx2.GasCost()
				} else {
					expectedEndSpongebobBalance = startingSpongebobBalance - tx1.Cost(false) - tx2.Cost(false) + database.BlockReward + database.TxFee
					expectedEndPatrickBalance = startingPatrickBalance + tx1.Value + tx2.Value + database.BlockReward + database.TxFee
				}

				if endSpongebobBalance != expectedEndSpongebobBalance {
					t.Errorf("Spongebob expected end balance is %d not %d", expectedEndSpongebobBalance, endSpongebobBalance)
				}

				if endPatrickBalance != expectedEndPatrickBalance {
					t.Errorf("Patrick expected end balance is %d not %d", expectedEndPatrickBalance, endPatrickBalance)
				}

				t.Logf("Starting Spongebob balance: %d", startingSpongebobBalance)
				t.Logf("Starting Patrick balance: %d", startingPatrickBalance)
				t.Logf("Ending Spongebob balance: %d", endSpongebobBalance)
				t.Logf("Ending Patrick balance: %d", endPatrickBalance)
			}()

			_ = n.Run(ctx)

			if n.state.LatestBlock().Header.Number != 2 {
				t.Fatal("was suppose to mine 2 pending TX into 2 valid blocks under 20m")
			}

			if len(n.pendingTXs) != 0 {
				t.Fatal("no pending TXs should be left to mine")
			}
		})
	}
}

func TestNode_MiningSpamTransactions(t *testing.T) {
	tc := []struct {
		name     string
		ForkTIP1 uint64
	}{
		{"Legacy", 35},  // Prior ForkTIP1 was activated on number 35.
		{"ForkTIP1", 0}, // To test new blocks when the ForkTIP1 is active.
	}

	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {

			spongebobBalance := uint(1000)
			patrickBalance := uint(0)
			minerBalance := uint(0)

			minerKey, err := wallet.NewRandomKey()
			if err != nil {
				t.Fatal(err)
			}
			miner := minerKey.Address
			dataDir, spongebob, patrick, err := setupTestNodeDir(spongebobBalance, tc.ForkTIP1)
			if err != nil {
				t.Fatal(err)
			}
			defer fs.RemoveDir(dataDir)

			n := New(dataDir, "127.0.0.1", 8085, miner, PeerNode{}, defaultTestMiningDifficulty)

			ctx, closeNode := context.WithCancel(context.Background())
			minerPeerNode := NewPeerNode("127.0.0.1", 8085, false, miner, true)

			txValue := uint(200)
			txCount := uint(4)
			spamTXs := make([]database.SignedTx, txCount)

			go func() {
				// Wait for the node to run.
				time.Sleep(time.Second)

				now := uint64(time.Now().Unix())
				// Schedule 4 transfers from Spongebob -> Patrick
				for i := uint(1); i <= txCount; i++ {
					txNonce := i
					tx := database.NewBaseTx(spongebob, patrick, txValue, txNonce, "")
					// Ensure every TX has a unique timestamp and the nonce 0 has oldest timestamp, nonce 1 younger timestamp etc
					tx.Time = now - uint64(txCount-i*100)

					if tc.name == "Legacy" {
						tx.Gas = 0
						tx.GasPrice = 0
					}

					signedTx, err := wallet.SignTxWithKeystoreAccount(tx, spongebob, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
					if err != nil {
						t.Fatal(err)
					}

					spamTXs[i-1] = signedTx
				}

				for _, tx := range spamTXs {
					_ = n.AddPendingTX(tx, minerPeerNode)
				}
			}()

			go func() {
				// Periodically check if we mined the block
				ticker := time.NewTicker(10 * time.Second)

				for {
					select {
					case <-ticker.C:
						if !n.state.LatestBlockHash().IsEmpty() {
							closeNode()
							return
						}
					}
				}
			}()

			_ = n.Run(ctx)

			var expectedSpongebobBalance uint
			var expectedPatrickBalance uint
			var expectedMinerBalance uint

			if n.state.IsTIP1Fork() {
				expectedSpongebobBalance = spongebobBalance
				expectedMinerBalance = minerBalance + database.BlockReward

				for _, tx := range spamTXs {
					expectedSpongebobBalance -= tx.Cost(true)
					expectedMinerBalance += tx.GasCost()
				}

				expectedPatrickBalance = patrickBalance + (txCount * txValue)
			} else {
				expectedSpongebobBalance = spongebobBalance - (txCount * txValue) - (txCount * database.TxFee)
				expectedPatrickBalance = patrickBalance + (txCount * txValue)
				expectedMinerBalance = minerBalance + database.BlockReward + (txCount * database.TxFee)
			}

			if n.state.Balances[spongebob] != expectedSpongebobBalance {
				t.Errorf("Spongebob balance is incorrect. Expected: %d. Got: %d", expectedSpongebobBalance, n.state.Balances[spongebob])
			}

			if n.state.Balances[patrick] != expectedPatrickBalance {
				t.Errorf("Patrick balance is incorrect. Expected: %d. Got: %d", expectedPatrickBalance, n.state.Balances[patrick])
			}

			if n.state.Balances[miner] != expectedMinerBalance {
				t.Errorf("Miner balance is incorrect. Expected: %d. Got: %d", expectedMinerBalance, n.state.Balances[miner])
			}

			t.Logf("Spongebob final balance: %d GC", n.state.Balances[spongebob])
			t.Logf("Patrick final balance: %d GC", n.state.Balances[patrick])
			t.Logf("Miner final balance: %d GC", n.state.Balances[miner])
		})
	}
}

func getTestDataDirPath() (string, error) {
	return ioutil.TempDir("/tmp", "test")
}

// Copy the pre-generated committed keystore files from this folder into the new testDataDirPath().
func copyKeystoreFilesIntoTestDataDirPath(dataDir string) error {
	spongebobSrcKs, err := os.Open(testKsSpongebobFile)
	if err != nil {
		return err
	}
	defer spongebobSrcKs.Close()

	ksDir := filepath.Join(wallet.GetKeystoreDirPath(dataDir))

	err = os.Mkdir(ksDir, 0777)
	if err != nil {
		return err
	}

	spongebobDstKs, err := os.Create(filepath.Join(ksDir, testKsSpongebobAccount))
	if err != nil {
		return err
	}
	defer spongebobDstKs.Close()

	_, err = io.Copy(spongebobDstKs, spongebobSrcKs)
	if err != nil {
		return err
	}

	patrickSrcKs, err := os.Open(testKsPatrickFile)
	if err != nil {
		return err
	}
	defer patrickSrcKs.Close()

	patrickDstKs, err := os.Create(filepath.Join(ksDir, testKsPatrickAccount))
	if err != nil {
		return err
	}
	defer patrickDstKs.Close()

	_, err = io.Copy(patrickDstKs, patrickSrcKs)
	if err != nil {
		return err
	}

	return nil
}

// setupTestNodeDir creates a default testing node directory with 2 keystore accounts.
func setupTestNodeDir(spongebobBalance uint, forkTip1 uint64) (dataDir string, spongebob, patrick common.Address, err error) {
	spongebob = database.NewAccount(testKsSpongebobAccount)
	patrick = database.NewAccount(testKsPatrickAccount)

	dataDir, err = getTestDataDirPath()
	if err != nil {
		return "", common.Address{}, common.Address{}, err
	}

	genesisBalances := make(map[common.Address]uint)
	genesisBalances[spongebob] = spongebobBalance
	genesis := database.Genesis{Balances: genesisBalances, ForkTIP1: forkTip1}
	genesisJson, err := json.Marshal(genesis)
	if err != nil {
		return "", common.Address{}, common.Address{}, err
	}

	err = database.InitDataDirIfNotExists(dataDir, genesisJson)
	if err != nil {
		return "", common.Address{}, common.Address{}, err
	}

	err = copyKeystoreFilesIntoTestDataDirPath(dataDir)
	if err != nil {
		return "", common.Address{}, common.Address{}, err
	}

	return dataDir, spongebob, patrick, nil
}
