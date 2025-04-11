package mongo

import (
	"context"
	"testing"

	"xiaoshiai.cn/common/controller"
	"xiaoshiai.cn/common/store"
)

type Message struct {
	store.ObjectMeta `json:",inline"`
}

func TestMongoStorage_Watch(t *testing.T) {
	ctx := context.Background()

	opt := NewDefaultMongoOptions()
	opt.Address = "bob-mongodb-headless.bob:27017"
	opt.Database = "bob"
	opt.Username = "root"
	opt.Password = "q1u9D20L0I"

	m, err := NewMongoStorage(ctx, GlobalObjectsScheme, opt)
	if err != nil {
		t.Errorf("NewMongoStorage() error = %v", err)
		return
	}

	source := controller.NewStoreSource(m, &Message{})

	c := controller.NewController("test", Noop{}).Watch(source)
	c.Run(ctx)
}

type Noop struct{}

func (Noop) Reconcile(ctx context.Context, key controller.ScopedKey) (controller.Result, error) {
	return controller.Result{}, nil
}
