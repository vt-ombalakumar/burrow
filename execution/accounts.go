package execution

import (
	"fmt"
	"sync"

	acm "github.com/hyperledger/burrow/account"
	"github.com/hyperledger/burrow/account/state"
	"github.com/hyperledger/burrow/keys"
	burrow_sync "github.com/hyperledger/burrow/sync"
)

type Accounts struct {
	burrow_sync.RingMutex
	state.Reader
	keyClient keys.KeyClient
}

type SigningAccount struct {
	acm.Account
	acm.Signer
}

type SequentialSigningAccount struct {
	accountLocker sync.Locker
	getter        func() (*SigningAccount, error)
}

func NewAccounts(reader state.Reader, keyClient keys.KeyClient, mutexCount int) *Accounts {
	return &Accounts{
		RingMutex: *burrow_sync.NewRingMutex(mutexCount),
		Reader:    reader,
		keyClient: keyClient,
	}
}
func (accs *Accounts) SigningAccount(address acm.Address, signer acm.Signer) (*SigningAccount, error) {
	account, err := state.GetMutableAccount(accs.Reader, address)
	if err != nil {
		return nil, err
	}
	// If the account is unknown to us return a zeroed account
	if account == nil {
		account = acm.ConcreteAccount{
			Address: address,
		}.MutableAccount()
	}
	pubKey, err := accs.keyClient.PublicKey(address)
	if err != nil {
		return nil, err
	}
	account.SetPublicKey(pubKey)
	return &SigningAccount{
		Account: account,
		Signer:  signer,
	}, nil
}

func (accs *Accounts) SequentialSigningAccount(address acm.Address) *SequentialSigningAccount {
	signer := keys.Signer(accs.keyClient, address)
	return &SequentialSigningAccount{
		accountLocker: accs.Mutex(address.Bytes()),
		getter: func() (*SigningAccount, error) {
			return accs.SigningAccount(address, signer)
		},
	}
}

func (accs *Accounts) SequentialSigningAccountFromPrivateKey(privateKeyBytes []byte) (*SequentialSigningAccount, error) {
	if len(privateKeyBytes) != 64 {
		return nil, fmt.Errorf("private key is not of the right length: %d\n", len(privateKeyBytes))
	}
	privateAccount, err := acm.GeneratePrivateAccountFromPrivateKeyBytes(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	return &SequentialSigningAccount{
		accountLocker: accs.Mutex(privateAccount.Address().Bytes()),
		getter: func() (*SigningAccount, error) {
			return accs.SigningAccount(privateAccount.Address(), privateAccount)
		},
	}, nil
}

type UnlockFunc func()

func (ssa *SequentialSigningAccount) Lock() (*SigningAccount, UnlockFunc, error) {
	ssa.accountLocker.Lock()
	account, err := ssa.getter()
	return account, ssa.accountLocker.Unlock, err
}
