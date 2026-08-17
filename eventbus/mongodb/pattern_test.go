package mongodb

import (
	"regexp"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestEventTypeFilter(t *testing.T) {
	tests := []struct {
		expression string
		value      string
		match      bool
	}{
		{expression: "order.created", value: "order.created", match: true},
		{expression: "order.foo*", value: "order.foobar", match: true},
		{expression: "order.foo*", value: "order.created", match: false},
		{expression: "order.created,updated", value: "order.updated", match: true},
		{expression: "order.{created,updated}", value: "order.updated", match: true},
		{expression: "order.**", value: "order", match: true},
		{expression: "order.**", value: "order.created.v1", match: true},
		{expression: "order.**", value: "order..v1", match: false},
	}
	for _, test := range tests {
		t.Run(test.expression+"/"+test.value, func(t *testing.T) {
			filter := eventTypeFilter(test.expression)
			matched := filter.Value == test.value
			if expression, ok := filter.Value.(primitive.Regex); ok {
				matched, _ = regexp.MatchString(expression.Pattern, test.value)
			}
			if matched != test.match {
				t.Fatalf("eventTypeFilter() match = %v, want %v", matched, test.match)
			}
		})
	}
}
