package mongo

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"xiaoshiai.cn/common/store"
)

type TestObject struct {
	store.ObjectMeta `json:",inline"`
	Status           TestObjectStatus `json:"status"`
}

type TestObjectStatus struct {
	Val   string   `json:"val"`
	Int   int      `json:"int"`
	Slice []string `json:"slice"`
}

func TestMergePatchToBsonUpdate(t *testing.T) {
	type args struct {
		data map[string]any
	}
	tests := []struct {
		name    string
		args    args
		want    bson.D
		wantErr bool
	}{
		{
			name: "defult",
			args: args{
				data: map[string]any{
					"alias": "test",
					"status": map[string]any{
						"val": "test",
					},
				},
			},
			want: bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "alias", Value: "test"},
					{Key: "status.val", Value: "test"},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergePatchToBsonUpdate(tt.args.data, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("MergePatchToBsonUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergePatchToBsonUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
