package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/gate/pkg/util/uuid"
)

var (
	ErrSkinNotFound   = errors.New("skin or player not found")
	ErrRateLimited    = errors.New("rate limited by skin provider")
	ErrInvalidURL     = errors.New("invalid skin image URL")
	ErrAllProvidersFail = errors.New("all skin providers failed to fetch skin")
)

// Fetcher resolves skins from external providers.
type Fetcher interface {
	FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error)
	FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error)
	FetchSkinByURL(ctx context.Context, imageURL string, skinModel model.SkinModel) (*model.SkinData, error)
}

// MultiProviderFetcher resolves skins with automatic failover across Mojang, Ashcon, PlayerDB, and Mineskin.
type MultiProviderFetcher struct {
	client    *http.Client
	mineskin  *MineskinFetcher
	mojang    *MojangFetcher
	ashcon    *AshconFetcher
	playerdb  *PlayerDBFetcher
	userAgent string
}

// NewMultiProviderFetcher constructs a high-resilience multi-provider skin fetcher.
func NewMultiProviderFetcher(mineskinApiKey string, timeout time.Duration) *MultiProviderFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := &http.Client{
		Timeout: timeout,
	}
	ua := "gate-skinschanger/1.0 (+https://github.com/andreisugu/gate-skinschanger)"

	return &MultiProviderFetcher{
		client:    httpClient,
		mojang:    NewMojangFetcher(httpClient, ua),
		ashcon:    NewAshconFetcher(httpClient, ua),
		playerdb:  NewPlayerDBFetcher(httpClient, ua),
		mineskin:  NewMineskinFetcher(httpClient, ua, mineskinApiKey),
		userAgent: ua,
	}
}

// FetchSkinByName attempts resolution via Mojang -> Ashcon -> PlayerDB.
func (m *MultiProviderFetcher) FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error) {
	// 1. Try Mojang official API
	skin, err := m.mojang.FetchSkinByName(ctx, name)
	if err == nil && skin != nil {
		return skin, nil
	}

	// 2. Try Ashcon API
	skin, err = m.ashcon.FetchSkinByName(ctx, name)
	if err == nil && skin != nil {
		return skin, nil
	}

	// 3. Try PlayerDB API
	skin, err = m.playerdb.FetchSkinByName(ctx, name)
	if err == nil && skin != nil {
		return skin, nil
	}

	return nil, fmt.Errorf("%w for '%s'", ErrAllProvidersFail, name)
}

// FetchSkinByUUID attempts resolution by Mojang UUID -> Ashcon -> PlayerDB.
func (m *MultiProviderFetcher) FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error) {
	// 1. Try Mojang session server
	skin, err := m.mojang.FetchSkinByUUID(ctx, id)
	if err == nil && skin != nil {
		return skin, nil
	}

	// 2. Try Ashcon API
	skin, err = m.ashcon.FetchSkinByUUID(ctx, id)
	if err == nil && skin != nil {
		return skin, nil
	}

	// 3. Try PlayerDB API
	skin, err = m.playerdb.FetchSkinByUUID(ctx, id)
	if err == nil && skin != nil {
		return skin, nil
	}

	return nil, fmt.Errorf("%w for UUID '%s'", ErrAllProvidersFail, id.String())
}

// FetchSkinByURL generates a signed skin from a direct image URL via Mineskin.
func (m *MultiProviderFetcher) FetchSkinByURL(ctx context.Context, imageURL string, skinModel model.SkinModel) (*model.SkinData, error) {
	return m.mineskin.FetchSkinByURL(ctx, imageURL, skinModel)
}
