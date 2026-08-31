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

type PlayerDBFetcher struct {
	client    *http.Client
	userAgent string
}

func NewPlayerDBFetcher(client *http.Client, userAgent string) *PlayerDBFetcher {
	return &PlayerDBFetcher{
		client:    client,
		userAgent: userAgent,
	}
}

type playerDBResponse struct {
	Code    string `json:"code"`
	Success bool   `json:"success"`
	Data    struct {
		Player struct {
			Username   string `json:"username"`
			ID         string `json:"id"`
			Properties []struct {
				Name      string `json:"name"`
				Value     string `json:"value"`
				Signature string `json:"signature"`
			} `json:"properties"`
		} `json:"player"`
	} `json:"data"`
}

func (f *PlayerDBFetcher) FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error) {
	return f.fetch(ctx, name)
}

func (f *PlayerDBFetcher) FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error) {
	return f.fetch(ctx, id.String())
}

func (f *PlayerDBFetcher) fetch(ctx context.Context, target string) (*model.SkinData, error) {
	reqURL := fmt.Sprintf("https://playerdb.co/api/player/minecraft/%s", target)
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
		return nil, fmt.Errorf("playerdb api returned status %d", resp.StatusCode)
	}

	var pdbResp playerDBResponse
	if err := json.NewDecoder(resp.Body).Decode(&pdbResp); err != nil {
		return nil, err
	}

	if !pdbResp.Success || pdbResp.Data.Player.Username == "" {
		return nil, ErrSkinNotFound
	}

	for _, prop := range pdbResp.Data.Player.Properties {
		if prop.Name == "textures" && prop.Value != "" {
			skin := &model.SkinData{
				Name:      pdbResp.Data.Player.Username,
				Value:     prop.Value,
				Signature: prop.Signature,
				FetchedAt: time.Now().UTC(),
			}
			skin.ExtractMetadata()
			return skin, nil
		}
	}

	return nil, ErrSkinNotFound
}
