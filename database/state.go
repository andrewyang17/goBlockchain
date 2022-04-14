package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/ethereum/go-ethereum/common"
)

const TxGas = 21
const TxGasPriceDefault = 1
const TxFee = uint(50)

type State struct {
	Balances      map[common.Address]uint
	Account2Nonce map[common.Address]uint

	dbFile *os.File

	latestBlock     Block
	latestBlockHash Hash
	hasGenesisBlock bool

	miningDifficulty uint
	forkTIP1         uint64

	HashCache   map[string]int64
	HeightCache map[uint64]int64
}

func NewStateFromDisk(dataDir string, miningDifficulty uint) (*State, error) {
	if err := InitDataDirIfNotExists(dataDir, []byte(genesisJson)); err != nil {
		return nil, err
	}

	gen, err := loadGenesis(getGenesisJsonFilePath(dataDir))
	if err != nil {
		return nil, err
	}

	balances := make(map[common.Address]uint)
	for account, balance := range gen.Balances {
		balances[account] = balance
	}

	account2nonce := make(map[common.Address]uint)

	dbFilepath := getBlocksDbFilePath(dataDir)
	dbFile, err := os.OpenFile(dbFilepath, os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(dbFile)

	state := &State{
		Balances:         balances,
		Account2Nonce:    account2nonce,
		dbFile:           dbFile,
		latestBlock:      Block{},
		latestBlockHash:  Hash{},
		hasGenesisBlock:  false,
		miningDifficulty: miningDifficulty,
		forkTIP1:         gen.ForkTIP1,
		HashCache:        map[string]int64{},
		HeightCache:      map[uint64]int64{},
	}

	var filePosition int64

	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		blockFsJson := scanner.Bytes()

		if len(blockFsJson) == 0 {
			break
		}

		var blockFs BlockFS

		if err := json.Unmarshal(blockFsJson, &blockFs); err != nil {
			return nil, err
		}

		if err := applyBlock(blockFs.Value, state); err != nil {
			return nil, err
		}

		// Set search caches
		state.HashCache[blockFs.Key.Hex()] = filePosition
		state.HeightCache[blockFs.Value.Header.Number] = filePosition
		filePosition += int64(len(blockFsJson)) + 1

		state.latestBlock = blockFs.Value
		state.latestBlockHash = blockFs.Key
		state.hasGenesisBlock = true
	}

	return state, nil
}

func (s *State) AddBlocks(blocks []Block) error {
	for _, block := range blocks {
		_, err := s.AddBlock(block)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *State) AddBlock(b Block) (Hash, error) {
	pendingState := s.Copy()

	err := applyBlock(b, &pendingState)
	if err != nil {
		return Hash{}, err
	}

	blockHash, err := b.Hash()
	if err != nil {
		return Hash{}, err
	}

	blockFs := BlockFS{
		Key:   blockHash,
		Value: b,
	}

	blockFsJson, err := json.Marshal(blockFs)
	if err != nil {
		return Hash{}, err
	}

	fmt.Printf("\nPersisting new Block to disk:\n")
	fmt.Printf("\t%s\n", blockFsJson)

	// Get file position for cache
	fs, _ := s.dbFile.Stat()
	filePosition := fs.Size() + 1

	_, err = s.dbFile.Write(append(blockFsJson, '\n'))
	if err != nil {
		return Hash{}, err
	}

	// Set search caches
	s.HashCache[blockFs.Key.Hex()] = filePosition
	s.HeightCache[blockFs.Value.Header.Number] = filePosition

	s.Balances = pendingState.Balances
	s.Account2Nonce = pendingState.Account2Nonce
	s.latestBlockHash = blockHash
	s.latestBlock = b
	s.hasGenesisBlock = true
	s.miningDifficulty = pendingState.miningDifficulty

	return blockHash, nil
}

func (s *State) NextBlockNumber() uint64 {
	if !s.hasGenesisBlock {
		return uint64(0)
	}

	return s.LatestBlock().Header.Number + 1
}

func (s *State) LatestBlock() Block {
	return s.latestBlock
}

func (s *State) LatestBlockHash() Hash {
	return s.latestBlockHash
}

func (s *State) GetNextAccountNonce(account common.Address) uint {
	return s.Account2Nonce[account] + 1
}

func (s *State) ChangeMiningDifficulty(newDifficulty uint) {
	s.miningDifficulty = newDifficulty
}

func (s *State) IsTIP1Fork() bool {
	return s.NextBlockNumber() >= s.forkTIP1
}

func (s *State) Close() error {
	return s.dbFile.Close()
}

func (s *State) Copy() State {
	var c State
	c.hasGenesisBlock = s.hasGenesisBlock
	c.latestBlock = s.latestBlock
	c.latestBlockHash = s.latestBlockHash
	c.Balances = make(map[common.Address]uint)
	c.Account2Nonce = make(map[common.Address]uint)
	c.miningDifficulty = s.miningDifficulty
	c.forkTIP1 = s.forkTIP1

	for acc, balance := range s.Balances {
		c.Balances[acc] = balance
	}

	for acc, nonce := range s.Account2Nonce {
		c.Account2Nonce[acc] = nonce
	}

	return c
}

// applyBlock verifies if a block can be added to the blockchain.
// Block metadata are verified as well as transactions within (sufficient balances).
func applyBlock(b Block, s *State) error {
	nextExpectedBlockNumber := s.latestBlock.Header.Number + 1

	if s.hasGenesisBlock && b.Header.Number != nextExpectedBlockNumber {
		return fmt.Errorf("next expected block must '%d' not '%d'", nextExpectedBlockNumber, b.Header.Number)
	}

	if s.hasGenesisBlock && s.latestBlock.Header.Number > 0 && !reflect.DeepEqual(b.Header.Parent, s.latestBlockHash) {
		return fmt.Errorf("next block parent hash must be '%x' not '%x'", s.latestBlockHash, b.Header.Parent)
	}

	hash, err := b.Hash()
	if err != nil {
		return err
	}

	if !IsBlockHashValid(hash, s.miningDifficulty) {
		return fmt.Errorf("invalid block hash %x", hash)
	}

	err = applyTXs(b.Txs, s)
	if err != nil {
		return err
	}

	s.Balances[b.Header.Miner] += BlockReward
	if s.IsTIP1Fork() {
		s.Balances[b.Header.Miner] += b.GasReward()
	} else {
		s.Balances[b.Header.Miner] += uint(len(b.Txs)) * TxFee
	}

	return nil
}

func applyTXs(txs []SignedTx, s *State) error {
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Time < txs[j].Time
	})

	for _, tx := range txs {
		if err := ApplyTx(tx, s); err != nil {
			return err
		}
	}
	return nil
}

func ApplyTx(tx SignedTx, s *State) error {
	err := ValidateTx(tx, s)
	if err != nil {
		return err
	}

	s.Balances[tx.From] -= tx.Cost(s.IsTIP1Fork())
	s.Balances[tx.To] += tx.Value

	s.Account2Nonce[tx.From] = tx.Nonce

	return nil
}

func ValidateTx(tx SignedTx, s *State) error {
	ok, err := tx.IsAuthentic()
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("wrong TX. Sender '%s' is forged", tx.From.String())
	}

	expectedNonce := s.GetNextAccountNonce(tx.From)
	if tx.Nonce != expectedNonce {
		return fmt.Errorf("wrong TX. Sender '%s' next nonce must be '%d', not '%d'", tx.From.String(), expectedNonce, tx.Nonce)
	}

	if s.IsTIP1Fork() {
		// For now we only have one type, transfer TXs, so all TXs must pay 21 gas like on Ethereum (21 000)
		if tx.Gas != TxGas {
			return fmt.Errorf("insufficient TX gas %v. required: %v", tx.Gas, TxGas)
		}

		if tx.GasPrice < TxGasPriceDefault {
			return fmt.Errorf("insufficient TX gasPrice %v. required at least: %v", tx.GasPrice, TxGasPriceDefault)
		}

	} else {
		// Prior to TIP1, a signed TX must NOT populate the Gas fields to prevent consensus from crashing
		// It's not enough to add this validation to http_routes.go because a TX could come from another node
		// that could modify its software and broadcast such a TX, it must be validated here too.
		if tx.Gas != 0 || tx.GasPrice != 0 {
			return fmt.Errorf("invalid TX. `Gas` and `GasPrice` can't be populated before TIP1 fork is active")
		}
	}

	if tx.Cost(s.IsTIP1Fork()) > s.Balances[tx.From] {
		return fmt.Errorf("wrong TX. Sender '%s' balance is %d GC. Tx cost is %d GC", tx.From.String(), s.Balances[tx.From], tx.Cost(s.IsTIP1Fork()))
	}

	return nil
}
