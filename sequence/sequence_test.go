package sequence_test

import (
	"context"

	"xiaoshiai.cn/common/sequence"
)

func ExampleAllocator() {
	var allocator sequence.Allocator
	_, _ = allocator.Next(context.Background(), "issues/repository-uid")
}
