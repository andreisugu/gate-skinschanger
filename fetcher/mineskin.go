package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
)

type MineskinFetcher struct {
	client    *http.Client
	userAgent string
	apiKey    string
}

func NewMineskinFetcher(client *http.Client, userAgent, apiKey string) *MineskinFetcher {
	return &MineskinFetcher{
		client:    client,
		userAgent: userAgent,
		apiKey:    apiKey,
	}
}

type mineskinRequest struct {
	URL        string `json:"url"`
	Model      string `json:"model,omitempty"`
	Visibility int    `json:"visibility,omitempty"` // 0 = public
}

type mineskinResponse struct {
	ID   int    `json:"id,omitempty"`
	UUID string `json:"uuid,omitempty"`
	Data struct {
		Texture struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
			URL       string `json:"url"`
		} `json:"texture"`
	} `json:"data"`
	// V2 format fallback
	Skin struct {
		Texture struct {
			Data struct {
				Value     string `json:"value"`
				Signature string `json:"signature"`
			} `json:"data"`
			URL string `json:"url"`
		} `json:"texture"`
	} `json:"skin,omitempty"`
	Error string `json:"error,omitempty"`
}

func (f *MineskinFetcher) FetchSkinByURL(ctx context.Context, imageURL string, skinModel model.SkinModel) (*model.SkinData, error) {
	if _, err := url.ParseRequestURI(imageURL); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	modelStr := "steve"
	if skinModel == model.ModelSlim {
		modelStr = "slim"
	}

	bodyData, err := json.Marshal(mineskinRequest{
		URL:   imageURL,
		Model: modelStr,
	})
	if err != nil {
		return nil, err
	}

	reqURL := "https://api.mineskin.org/generate/url"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", f.userAgent)
	if f.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+f.apiKey)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mineskin api returned status %d", resp.StatusCode)
	}

	var msResp mineskinResponse
	if err := json.NewDecoder(resp.Body).Decode(&msResp); err != nil {
		return nil, err
	}

	val := msResp.Data.Texture.Value
	sig := msResp.Data.Texture.Signature
	if val == "" && msResp.Skin.Texture.Data.Value != "" {
		val = msResp.Skin.Texture.Data.Value
		sig = msResp.Skin.Texture.Data.Signature
	}

	if val == "" {
		if msResp.Error != "" {
			return nil, fmt.Errorf("mineskin error: %s", msResp.Error)
		}
		return nil, fmt.Errorf("mineskin returned empty texture value")
	}

	skin := &model.SkinData{
		Name:      "custom_url",
		Value:     val,
		Signature: sig,
		Model:     skinModel,
		FetchedAt: time.Now().UTC(),
	}
	skin.ExtractMetadata()
	return skin, nil
}
