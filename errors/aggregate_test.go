package errors

import (
	stderrors "errors"
	"testing"
)

type aggregateTestError struct {
	value string
}

func (err *aggregateTestError) Error() string {
	return err.value
}

func TestAggregateSupportsErrorsAs(t *testing.T) {
	want := &aggregateTestError{value: "typed failure"}
	err := NewAggregate([]error{stderrors.New("other failure"), want})

	got := &aggregateTestError{}
	if !stderrors.As(err, &got) {
		t.Fatalf("errors.As(%v) = false, want true", err)
	}
	if got != want {
		t.Fatalf("errors.As() = %#v, want %#v", got, want)
	}
}
