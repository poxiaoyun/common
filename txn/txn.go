package txn

// Package txn provides a simple transaction mechanism for managing operations
// that need to be committed or reverted as a group.

// ctx is the Context shares amoung transactions.

// Transaction represents something that may need to be finalized on success or
// failure of the larger transaction.
type Transaction[T any] interface {
	// Commit tells the transaction to finalize any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Commit(ctx T) error

	// Revert tells the transaction to abandon or undo any changes it may have
	// pending.  This cannot fail, so errors must be handled internally.
	Revert(ctx T) error
}

// CallbackTransaction is a transaction which calls arbitrary functions.
type CallbackTransaction[T any] struct {
	CommitFunc func(T) error
	RevertFunc func(T) error
}

func (cb CallbackTransaction[T]) Commit(ctx T) error {
	if cb.CommitFunc != nil {
		return cb.CommitFunc(ctx)
	}
	return nil
}

func (cb CallbackTransaction[T]) Revert(ctx T) error {
	if cb.RevertFunc != nil {
		return cb.RevertFunc(ctx)
	}
	return nil
}

func Execute[T any](ctx T, txns ...Transaction[T]) error {
	for i, txn := range txns {
		if err := txn.Commit(ctx); err != nil {
			for j := i - 1; j >= 0; j-- {
				txns[j].Revert(ctx)
			}
			return err
		}
	}
	return nil
}
