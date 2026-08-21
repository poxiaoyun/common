package asset_test

import (
	"encoding/json"
	"strings"
	"testing"

	"xiaoshiai.cn/common/asset"
)

func TestAssetJSONUsesTarget(t *testing.T) {
	data, err := json.Marshal(asset.Asset{
		Target: asset.Target{Kind: "application", Name: "cloud:database"},
		Name:   "icon",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["resource"]; exists {
		t.Fatalf("Asset JSON contains legacy resource field: %s", data)
	}
	target, ok := decoded["target"].(map[string]any)
	if !ok || target["kind"] != "application" || target["name"] != "cloud:database" {
		t.Fatalf("Asset JSON target = %#v", decoded["target"])
	}
}

func TestValidateBlob(t *testing.T) {
	tests := []struct {
		name      string
		input     asset.Blob
		wantError bool
	}{
		{
			name: "content",
			input: asset.Blob{
				Content:       strings.NewReader("content"),
				ContentType:   "text/plain",
				ContentLength: 7,
			},
		},
		{
			name: "link",
			input: asset.Blob{
				Link:        &asset.Link{URL: "https://objects.example/icon"},
				ContentType: "image/png",
			},
		},
		{name: "missing source", input: asset.Blob{ContentType: "text/plain"}, wantError: true},
		{
			name: "multiple sources",
			input: asset.Blob{
				Content:     strings.NewReader("content"),
				Link:        &asset.Link{URL: "https://objects.example/icon"},
				ContentType: "text/plain",
			},
			wantError: true,
		},
		{name: "missing content type", input: asset.Blob{Content: strings.NewReader("content")}, wantError: true},
		{
			name: "negative content length",
			input: asset.Blob{
				Content:       strings.NewReader("content"),
				ContentType:   "text/plain",
				ContentLength: -1,
			},
			wantError: true,
		},
		{name: "missing link URL", input: asset.Blob{Link: &asset.Link{}, ContentType: "image/png"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := asset.ValidateBlob(test.input)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateBlob(%#v) error = %v, wantError = %t", test.input, err, test.wantError)
			}
		})
	}
}

func TestValidateTargetAndAssetName(t *testing.T) {
	tests := []struct {
		name   string
		target asset.Target
		asset  string
		valid  bool
	}{
		{name: "plain target", target: asset.Target{Kind: "user", Name: "alice"}, asset: "avatar.png", valid: true},
		{name: "scoped target", target: asset.Target{Kind: "application", Name: "cloud:database"}, asset: "icon", valid: true},
		{name: "empty kind", target: asset.Target{Name: "alice"}, asset: "icon"},
		{name: "three target segments", target: asset.Target{Kind: "application", Name: "a:b:c"}, asset: "icon"},
		{name: "invalid asset", target: asset.Target{Kind: "user", Name: "alice"}, asset: "bad/name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := asset.Validate(test.target, test.asset)
			if (err == nil) != test.valid {
				t.Fatalf("Validate() error = %v, valid = %t", err, test.valid)
			}
		})
	}
}
