package node

import (
	"encoding/base64"
	"encoding/json"
	"github.com/andrewyang17/goBlockchain/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/andrewyang17/goBlockchain/wallet"
	"github.com/ethereum/go-ethereum/common"
)

func TestNode_MempoolView(t *testing.T) {
	spongebob := database.NewAccount(testKsSpongebobAccount)
	patrick := database.NewAccount(testKsPatrickAccount)

	// Test cases
	poolLen := 3
	txn3From := patrick

	dataDir, err := getTestDataDirPath()
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(dataDir)

	genesisBalances := make(map[common.Address]uint)
	genesisBalances[spongebob] = 1000000
	genesisBalances[patrick] = 1000000

	genesis := database.Genesis{Balances: genesisBalances}
	genesisJson, err := json.Marshal(genesis)
	if err != nil {
		t.Fatal(err)
	}

	err = database.InitDataDirIfNotExists(dataDir, genesisJson)
	if err != nil {
		t.Fatal(err)
	}

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

	n := New(dataDir, nInfo.IP, nInfo.Port, patrick, nInfo, DefaultMiningDifficulty)

	state, err := database.NewStateFromDisk(n.dataDir, n.miningDifficulty)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()

	n.state = state

	pendingState := state.Copy()
	n.pendingState = &pendingState

	tx1 := database.NewBaseTx(spongebob, patrick, 1, 1, "")
	tx2 := database.NewBaseTx(spongebob, patrick, 2, 2, "")
	tx3 := database.NewBaseTx(patrick, spongebob, 1, 1, "")

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

	signedTx3, err := wallet.SignTxWithKeystoreAccount(tx3, patrick, testKsAccountsPwd, wallet.GetKeystoreDirPath(dataDir))
	if err != nil {
		t.Error(err)
		return
	}

	// Add 3 new TXs
	err = n.AddPendingTX(signedTx1, nInfo)
	if err != nil {
		t.Fatal(err)
	}
	err = n.AddPendingTX(signedTx2, nInfo)
	if err != nil {
		t.Fatal(err)
	}
	err = n.AddPendingTX(signedTx3, nInfo)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, endpointMempoolViewer, nil)

	func(w http.ResponseWriter, r *http.Request, node *Node) {
		mempoolViewHandler(w, r, node.pendingTXs)
	}(rr, req, n)

	if rr.Code != http.StatusOK {
		t.Fatal("unexpected status code: ", rr.Code, rr.Body.String())
	}

	var resp map[string]database.SignedTx
	dec := json.NewDecoder(rr.Body)
	err = dec.Decode(&resp)
	if err != nil {
		t.Fatal("error decoding", err)
	}

	// check pool length
	if len(resp) != poolLen {
		t.Fatalf("mempool viewer reponse len wrong, got %v; want %v", len(resp), poolLen)
	}

	for _, v := range resp {
		// check for third case
		if v.From.Hex() == txn3From.Hex() {
			if !reflect.DeepEqual(signedTx3.Sig, v.Sig) {
				t.Errorf("invalid signature for txn, got %q, want %q", base64.StdEncoding.EncodeToString(v.Sig), base64.StdEncoding.EncodeToString(signedTx3.Sig))
			}
		}
	}
}
