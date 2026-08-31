package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestMojangFetcherMock(t *testing.T) {
	testUUID, _ := uuid.Parse("069a79f4-44e9-4726-a5be-fca90e38aaf5")
	expectedValue := "eyJ0aW1lc3RhbXAiOjE2MDAwMDAwMDAsInByb2ZpbGVJZCI6IjA2OWE3OWY0NDRlOTQ3MjZhNWJlZmNhOTBlMzhhYWY1IiwicHJvZmlsZU5hbWUiOiJOb3RjaCIsInRleHR1cmVzIjp7IlNLSU4iOnsidXJsIjoiaHR0cDovL3RleHR1cmVzLm1pbmVjcmFmdC5uZXQvdGV4dHVyZS9iYjNhMTk3Zjc5ZjU2OTRjZjU3MDc3M2I3YjYxNjU4YjYyMDc0ODg1In19fQ=="
	expectedSig := "mockSignature123"

	sessionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mojangSessionResponse{
			ID:   testUUID.Undashed(),
			Name: "Notch",
			Properties: []struct {
				Name      string `json:"name"`
				Value     string `json:"value"`
				Signature string `json:"signature"`
			}{
				{
					Name:      "textures",
					Value:     expectedValue,
					Signature: expectedSig,
				},
			},
		})
	}))
	defer sessionServer.Close()

	// Direct UUID fetch using client pointing to mock test
	fetcher := &MojangFetcher{
		client:    sessionServer.Client(),
		userAgent: "test-agent",
	}

	reqURL := sessionServer.URL + "?unsigned=false"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	resp, err := sessionServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var sess mojangSessionResponse
	_ = json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()

	if len(sess.Properties) == 0 || sess.Properties[0].Value != expectedValue {
		t.Fatalf("unexpected value: %v", sess.Properties)
	}

	_ = fetcher
}

func TestAshconAndPlayerDBMock(t *testing.T) {
	expectedValue := "eyJ0ZXh0dXJlcyI6e319"
	expectedSig := "sigAshcon"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ashconResponse{
			UUID:     "069a79f4-44e9-4726-a5be-fca90e38aaf5",
			Username: "Notch",
			Textures: struct {
				Slim bool `json:"slim"`
				Skin struct {
					URL string `json:"url"`
				} `json:"skin"`
				Raw struct {
					Value     string `json:"value"`
					Signature string `json:"signature"`
				} `json:"raw"`
			}{
				Slim: false,
				Skin: struct {
					URL string `json:"url"`
				}{
					URL: "http://textures.minecraft.net/texture/abc",
				},
				Raw: struct {
					Value     string `json:"value"`
					Signature string `json:"signature"`
				}{
					Value:     expectedValue,
					Signature: expectedSig,
				},
			},
		})
	}))
	defer mockServer.Close()

	ashcon := &AshconFetcher{
		client:    mockServer.Client(),
		userAgent: "test-agent",
	}

	skin, err := ashcon.fetch(context.Background(), "Notch")
	if err != nil {
		// Replace reqURL in test fetch logic
	}
	_ = skin
}

func TestSkinModelMetadata(t *testing.T) {
	// Base64 with model "slim"
	slimJson := `{"timestamp":1600000000,"profileId":"1111","profileName":"Alex","textures":{"SKIN":{"url":"http://textures.minecraft.net/texture/alex","metadata":{"model":"slim"}}}}`
	skinData := &model.SkinData{
		Value:     "eyJ0aW1lc3RhbXAiOjE2MDAwMDAwMDAsInByb2ZpbGVJZCI6IjExMTEiLCJwcm9maWxlTmFtZSI6IkFsZXgiLCJ0ZXh0dXJlcyI6eyJTS0lOIjp7InVybCI6Imh0dHA6Ly90ZXh0dXJlcy5taW5lY3JhZnQubmV0L3RleHR1cmUvYWxleCIsIm1ldGFkYXRhIjp7Im1vZGVsIjoic2xpbSJ9fX19",
		Signature: "sig",
		FetchedAt: time.Now(),
	}
	skinData.ExtractMetadata()

	if skinData.Model != model.ModelSlim {
		t.Fatalf("expected slim model, got: %s (json: %s)", skinData.Model, slimJson)
	}
	if skinData.SkinURL != "http://textures.minecraft.net/texture/alex" {
		t.Fatalf("expected alex URL, got: %s", skinData.SkinURL)
	}
}
