package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
)

func GetBlocksAfter(blockHash Hash, dataDir string) ([]Block, error) {
	f, err := os.OpenFile(getBlocksDbFilePath(dataDir), os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}

	blocks := make([]Block, 0)

	var shouldStartCollecting bool

	if reflect.DeepEqual(blockHash, Hash{}) {
		shouldStartCollecting = true
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		var blockFS BlockFS

		err = json.Unmarshal(scanner.Bytes(), &blockFS)
		if err != nil {
			return nil, err
		}

		if shouldStartCollecting {
			blocks = append(blocks, blockFS.Value)
			continue
		}

		if blockHash == blockFS.Key {
			shouldStartCollecting = true
		}
	}

	return blocks, nil
}

// GetBlockByHeightOrHash returns the requested block by hash or height.
// It uses cached data in the State struct (HashCache / HeightCache)
func GetBlockByHeightOrHash(state *State, height uint64, hash, dataDir string) (BlockFS, error) {
	var block BlockFS

	key, ok := state.HeightCache[height]
	if hash != "" {
		key, ok = state.HashCache[hash]
	}

	if !ok {
		if hash != "" {
			return block, fmt.Errorf("invalid hash: '%v", hash)
		}
		return block, fmt.Errorf("invalid height: '%v", height)
	}

	f, err := os.OpenFile(getBlocksDbFilePath(dataDir), os.O_RDONLY, 0600)
	if err != nil {
		return block, err
	}
	defer f.Close()

	_, err = f.Seek(key, 0)
	if err != nil {
		return block, err
	}

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return block, err
		}

		err = json.Unmarshal(scanner.Bytes(), &block)
		if err != nil {
			return block, err
		}
	}

	return block, nil
}
