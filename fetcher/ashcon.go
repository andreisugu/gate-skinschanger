package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/gate/pkg/util/uuid"
)

type AshconFetcher struct {
	client    *http.Client
	userAgent string
}

func NewAshconFetcher(client *http.Client, userAgent string) *AshconFetcher {
	return &AshconFetcher{
		client:    client,
		userAgent: userAgent,
	}
}

type ashconResponse struct {
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Textures struct {
		Slim bool `json:"slim"`
		Skin struct {
			URL string `json:"url"`
		} `json:"skin"`
		Raw struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"raw"`
	} `json:"textures"`
}

func (f *AshconFetcher) FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error) {
	return f.fetch(ctx, name)
}

func (f *AshconFetcher) FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error) {
	return f.fetch(ctx, id.String())
}

func (f *AshconFetcher) fetch(ctx context.Context, target string) (*model.SkinData, error) {
	reqURL := fmt.Sprintf("https://api.ashcon.app/mojang/v2/user/%s", target)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSkinNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ashcon api returned status %d", resp.StatusCode)
	}

	var aResp ashconResponse
	if err := json.NewDecoder(resp.Body).Decode(&aResp); err != nil {
		return nil, err
	}

	if aResp.Textures.Raw.Value == "" {
		return nil, ErrSkinNotFound
	}

	skinModel := model.ModelClassic
	if aResp.Textures.Slim {
		skinModel = model.ModelSlim
	}

	skin := &model.SkinData{
		Name:      aResp.Username,
		Value:     aResp.Textures.Raw.Value,
		Signature: aResp.Textures.Raw.Signature,
		Model:     skinModel,
		SkinURL:   aResp.Textures.Skin.URL,
		FetchedAt: time.Now().UTC(),
	}
	return skin, nil
}
