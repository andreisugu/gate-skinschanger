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

type MojangFetcher struct {
	client    *http.Client
	userAgent string
}

func NewMojangFetcher(client *http.Client, userAgent string) *MojangFetcher {
	return &MojangFetcher{
		client:    client,
		userAgent: userAgent,
	}
}

type mojangProfileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type mojangSessionResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		Signature string `json:"signature"`
	} `json:"properties"`
}

func (f *MojangFetcher) FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error) {
	reqURL := fmt.Sprintf("https://api.mojang.com/users/profiles/minecraft/%s", name)
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

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, ErrSkinNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mojang api returned status %d", resp.StatusCode)
	}

	var prof mojangProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&prof); err != nil {
		return nil, err
	}

	parsedUUID, err := uuid.Parse(prof.ID)
	if err != nil {
		return nil, err
	}

	return f.FetchSkinByUUID(ctx, parsedUUID)
}

func (f *MojangFetcher) FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error) {
	reqURL := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/profile/%s?unsigned=false", id.Undashed())
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

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, ErrSkinNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mojang session server returned status %d", resp.StatusCode)
	}

	var sess mojangSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sess); err != nil {
		return nil, err
	}

	for _, prop := range sess.Properties {
		if prop.Name == "textures" && prop.Value != "" {
			skin := &model.SkinData{
				Name:      sess.Name,
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
