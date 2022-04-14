package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/andrewyang17/goBlockchain/database"
	"github.com/andrewyang17/goBlockchain/fs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

const testKeystoreAccountsPassword = "security123"

func TestSignCryptoParam(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("testing")

	sig, err := Sign(msg, privKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(sig) != crypto.SignatureLength {
		t.Fatal(fmt.Errorf("wrong size for signature: got %d, want %d", len(sig), crypto.SignatureLength))
	}
}

func TestSign(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pubKey := privKey.PublicKey
	pubKeyBytes := elliptic.Marshal(crypto.S256(), pubKey.X, pubKey.Y)
	pubKeyBytesHash := crypto.Keccak256(pubKeyBytes[1:])
	account := common.BytesToAddress(pubKeyBytesHash[12:])

	msg := []byte("testing")

	sig, err := Sign(msg, privKey)
	if err != nil {
		t.Fatal(err)
	}

	recoveredPubKey, err := Verify(msg, sig)
	if err != nil {
		t.Fatal(err)
	}

	recoveredPubKeyBytes := elliptic.Marshal(crypto.S256(), recoveredPubKey.X, recoveredPubKey.Y)
	recoveredPubKeyBytesHash := crypto.Keccak256(recoveredPubKeyBytes[1:])
	recoveredAccount := common.BytesToAddress(recoveredPubKeyBytesHash[12:])

	if account.Hex() != recoveredAccount.Hex() {
		t.Fatalf("msg was signed by account %s but signature recovery produced an account %s", account.Hex(), recoveredAccount.Hex())
	}
}

func TestSignTxWithKeystoreAccount(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "wallet_test")
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(tmpDir)

	spongebob, err := NewKeystoreAccount(tmpDir, testKeystoreAccountsPassword)
	if err != nil {
		t.Fatal(err)
	}

	patrick, err := NewKeystoreAccount(tmpDir, testKeystoreAccountsPassword)
	if err != nil {
		t.Fatal(err)
	}

	tx := database.NewBaseTx(spongebob, patrick, 100, 1, "")

	signedTx, err := SignTxWithKeystoreAccount(tx, spongebob, testKeystoreAccountsPassword, GetKeystoreDirPath(tmpDir))
	if err != nil {
		t.Fatal(err)
	}

	ok, err := signedTx.IsAuthentic()
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("the TX was signed by 'from' account and should have been authentic")
	}

	signedTxJson, err := json.Marshal(signedTx)
	if err != nil {
		t.Fatal(err)
	}

	var signedTxUnmarshaled database.SignedTx
	err = json.Unmarshal(signedTxJson, &signedTxUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, signedTx, signedTxUnmarshaled)
}

func TestSignForgedTxWithKeystoreAccount(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "wallet_test")
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveDir(tmpDir)

	hacker, err := NewKeystoreAccount(tmpDir, testKeystoreAccountsPassword)
	if err != nil {
		t.Error(err)
		return
	}

	patrick, err := NewKeystoreAccount(tmpDir, testKeystoreAccountsPassword)
	if err != nil {
		t.Error(err)
		return
	}

	forgedTx := database.NewBaseTx(patrick, hacker, 100, 1, "")

	signedTx, err := SignTxWithKeystoreAccount(forgedTx, hacker, testKeystoreAccountsPassword, GetKeystoreDirPath(tmpDir))
	if err != nil {
		t.Error(err)
		return
	}

	ok, err := signedTx.IsAuthentic()
	if err != nil {
		t.Error(err)
		return
	}

	if ok {
		t.Fatal("the TX 'from' attribute was forged and should have not be authentic")
	}
}
