package wallet

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

const keystoreDirName = "keystore"

func GetKeystoreDirPath(dataDir string) string {
	return filepath.Join(dataDir, keystoreDirName)
}

func NewKeystoreAccount(dataDir, password string) (common.Address, error) {
	ks := keystore.NewKeyStore(GetKeystoreDirPath(dataDir), keystore.StandardScryptN, keystore.StandardScryptP)
	acc, err := ks.NewAccount(password)
	if err != nil {
		return common.Address{}, err
	}

	return acc.Address, nil
}

func SignTxWithKeystoreAccount(tx database.Tx, acc common.Address, pwd, keystoreDir string) (database.SignedTx, error) {
	ks := keystore.NewKeyStore(keystoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	ksAccount, err := ks.Find(accounts.Account{Address: acc})
	if err != nil {
		return database.SignedTx{}, err
	}

	ksAccountJson, err := ioutil.ReadFile(ksAccount.URL.Path)
	if err != nil {
		return database.SignedTx{}, err
	}

	key, err := keystore.DecryptKey(ksAccountJson, pwd)
	if err != nil {
		return database.SignedTx{}, err
	}

	signedTx, err := SignTx(tx, key.PrivateKey)
	if err != nil {
		return database.SignedTx{}, err
	}

	return signedTx, nil
}

func SignTx(tx database.Tx, privKey *ecdsa.PrivateKey) (database.SignedTx, error) {
	rawTx, err := tx.Encode()
	if err != nil {
		return database.SignedTx{}, err
	}

	sig, err := Sign(rawTx, privKey)
	if err != nil {
		return database.SignedTx{}, err
	}

	return database.NewSignedTx(tx, sig), nil
}

func Sign(msg []byte, privKey *ecdsa.PrivateKey) (sign []byte, err error) {
	msgHash := sha256.Sum256(msg)

	sig, err := crypto.Sign(msgHash[:], privKey)
	if err != nil {
		return nil, err
	}

	if len(sig) != crypto.SignatureLength {
		return nil, fmt.Errorf("wrong size for signature: got %d, want %d", len(sig), crypto.SignatureLength)
	}

	return sig, nil
}

func Verify(msg, sig []byte) (*ecdsa.PublicKey, error) {
	msgHash := sha256.Sum256(msg)

	recoveredPubKey, err := crypto.SigToPub(msgHash[:], sig)
	if err != nil {
		return nil, fmt.Errorf("unable to verify message signature: %s", err.Error())
	}

	return recoveredPubKey, nil
}

func NewRandomKey() (*keystore.Key, error) {
	privKeyECDSA, err := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	id, _ := uuid.NewRandom()

	key := &keystore.Key{
		Id:         id,
		Address:    crypto.PubkeyToAddress(privKeyECDSA.PublicKey),
		PrivateKey: privKeyECDSA,
	}

	return key, nil
}