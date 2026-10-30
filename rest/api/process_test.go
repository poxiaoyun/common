package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestOnWritesExplicitStatusAndBody(t *testing.T) {
	server := api.New().
		Group(api.NewGroup("/v1").
			Route(api.POST("/orders").
				To(func(w http.ResponseWriter, r *http.Request) {
					api.On(w, r, func(context.Context) (any, error) {
						return api.Response(http.StatusCreated, map[string]string{"id": "order-a"}), nil
					})
				}))).
		Build()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/orders", nil))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "order-a" {
		t.Fatalf("body = %v", body)
	}
}
