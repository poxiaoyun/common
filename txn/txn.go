package txn

// Package txn provides a simple transaction mechanism for managing operations
// that need to be committed or reverted as a group.

// Transaction represents something that may need to be finalized on success or
// failure of the larger transaction.
type Transaction interface {
	// Commit tells the transaction to finalize any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Commit() error

	// Revert tells the transaction to abandon or undo any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Revert() error
}

// CallbackTransaction is a transaction which calls arbitrary functions.
type CallbackTransaction struct {
	CommitFunc func() error
	RevertFunc func() error
}

func (cb CallbackTransaction) Commit() error {
	if cb.CommitFunc != nil {
		return cb.CommitFunc()
	}
	return nil
}

func (cb CallbackTransaction) Revert() error {
	if cb.RevertFunc != nil {
		return cb.RevertFunc()
	}
	return nil
}

func Execute(txns ...Transaction) error {
	for i, txn := range txns {
		if err := txn.Commit(); err != nil {
			for j := i - 1; j >= 0; j-- {
				txns[j].Revert()
			}
			return err
		}
	}
	return nil
}
