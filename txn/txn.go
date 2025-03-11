package txn

// Transaction represents something that may need to be finalized on success or
// failure of the larger transaction.
type Transaction interface {
	// Commit tells the transaction to finalize any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Commit()

	// Revert tells the transaction to abandon or undo any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Revert()
}

// CallbackTransaction is a transaction which calls arbitrary functions.
type CallbackTransaction struct {
	CommitFunc func()
	RevertFunc func()
}

func (cb CallbackTransaction) Commit() {
	if cb.CommitFunc != nil {
		cb.CommitFunc()
	}
}

func (cb CallbackTransaction) Revert() {
	if cb.RevertFunc != nil {
		cb.RevertFunc()
	}
}

// metaTransaction is a collection of transactions.
type Transactions []Transaction

func (mt Transactions) Commit() {
	for _, t := range mt {
		t.Commit()
	}
}

func (mt Transactions) Revert() {
	for _, t := range mt {
		t.Revert()
	}
}
