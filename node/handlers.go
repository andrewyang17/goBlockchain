package node

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/andrewyang17/goBlockchain/wallet"
	"github.com/ethereum/go-ethereum/common"
)

func listBalancesHandler(w http.ResponseWriter, r *http.Request, state *database.State) {
	enableCors(&w)

	writeRes(w, BalancesRes{
		Hash:     state.LatestBlockHash(),
		Balances: state.Balances,
	})
}

func txAddHandler(w http.ResponseWriter, r *http.Request, node *Node) {
	req := TxAddReq{}
	err := readReq(r, &req)
	if err != nil {
		writeErrRes(w, err)
		return
	}

	from := database.NewAccount(req.From)

	if from.String() == common.HexToAddress("").String() {
		writeErrRes(w, fmt.Errorf("%s is an invalid 'from' sender", from.String()))
		return
	}

	if req.FromPwd == "" {
		writeErrRes(w, fmt.Errorf("password to decrypt the %s acount is required. 'from_pwd' is empty", from.String()))
		return
	}

	if req.Gas == 0 {
		req.Gas = 1
	}

	if req.GasPrice == 0 {
		req.GasPrice = 21
	}

	nonce := node.state.GetNextAccountNonce(from)
	tx := database.NewTx(from, database.NewAccount(req.To), req.Gas, req.GasPrice, req.Value, nonce, req.Data)

	signedTx, err := wallet.SignTxWithKeystoreAccount(tx, from, req.FromPwd, wallet.GetKeystoreDirPath(node.dataDir))
	if err != nil {
		writeErrRes(w, err)
		return
	}

	err = node.AddPendingTX(signedTx, node.info)
	if err != nil {
		writeErrRes(w, err)
	}

	writeRes(w, TxAddRes{Success: true})
}

func statusHandler(w http.ResponseWriter, r *http.Request, node *Node) {
	enableCors(&w)

	res := StatusRes{
		Hash:       node.state.LatestBlockHash(),
		Number:     node.state.LatestBlock().Header.Number,
		KnownPeers: node.knownPeers,
		PendingTxs: node.getPendingTXsAsArray(),
	}

	writeRes(w, res)
}

func syncHandler(w http.ResponseWriter, r *http.Request, node *Node) {
	reqHash := r.URL.Query().Get(endpointSyncQueryKeyFromBlock)

	hash := database.Hash{}
	err := hash.UnmarshalText([]byte(reqHash))
	if err != nil {
		writeErrRes(w, err)
		return
	}

	blocks, err := database.GetBlocksAfter(hash, node.dataDir)
	if err != nil {
		writeErrRes(w, err)
		return
	}

	writeRes(w, SyncRes{Blocks: blocks})
}

func addPeerHandler(w http.ResponseWriter, r *http.Request, node *Node) {
	peerIP := r.URL.Query().Get(endpointAddPeerQueryKeyIP)
	peerPortRaw := r.URL.Query().Get(endpointAddPeerQueryKeyPort)
	minerRaw := r.URL.Query().Get(endpointAddPeerQueryKeyMiner)

	peerPort, err := strconv.ParseUint(peerPortRaw, 10, 32)
	if err != nil {
		writeRes(w, AddPeerRes{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	peer := NewPeerNode(peerIP, peerPort, false, database.NewAccount(minerRaw), true)
	node.AddPeer(peer)

	fmt.Printf("Peer '%s' was added into knownPeers\n", peer.TcpAddress())

	writeRes(w, AddPeerRes{Success: true})
}

func getBlockByNumberOrHashHandler(w http.ResponseWriter, r *http.Request, node *Node) {
	enableCors(&w)

	errorParamsRequired := errors.New("height or hash param is required")

	params := strings.Split(r.URL.Path, "/")[1:]
	if len(params) < 2 {
		writeErrRes(w, errorParamsRequired)
		return
	}

	p := strings.TrimSpace(params[1])
	if len(p) == 0 {
		writeErrRes(w, errorParamsRequired)
		return
	}

	var hash string

	height, err := strconv.ParseUint(p, 10, 64)
	if err != nil {
		hash = p
	}

	block, err := database.GetBlockByHeightOrHash(node.state, height, hash, node.dataDir)
	if err != nil {
		writeErrRes(w, err)
		return
	}

	writeRes(w, block)
}

func mempoolViewHandler(w http.ResponseWriter, r *http.Request, txs map[string]database.SignedTx) {
	enableCors(&w)

	writeRes(w, txs)
}
