package manager

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/andreisugu/gate-skinschanger/fetcher"
	"github.com/andreisugu/gate-skinschanger/model"
	"github.com/andreisugu/gate-skinschanger/storage"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

type mockFetcher struct {
	skins map[string]*model.SkinData
}

func (m *mockFetcher) FetchSkinByName(ctx context.Context, name string) (*model.SkinData, error) {
	if s, ok := m.skins[name]; ok {
		return s, nil
	}
	return nil, fetcher.ErrSkinNotFound
}

func (m *mockFetcher) FetchSkinByUUID(ctx context.Context, id uuid.UUID) (*model.SkinData, error) {
	for _, s := range m.skins {
		return s, nil
	}
	return nil, fetcher.ErrSkinNotFound
}

func (m *mockFetcher) FetchSkinByURL(ctx context.Context, imageURL string, skinModel model.SkinModel) (*model.SkinData, error) {
	return &model.SkinData{
		Name:      "custom_url",
		Value:     "urlValue123",
		Signature: "urlSig123",
		Model:     skinModel,
		FetchedAt: time.Now(),
	}, nil
}

func TestManagerProfileProcessing(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mgr_test_*")
	defer os.RemoveAll(tmpDir)

	store := storage.NewStorage(tmpDir, time.Hour)
	_ = store.Load()

	notchSkin := &model.SkinData{
		Name:      "Notch",
		Value:     "notchTexturesValue",
		Signature: "notchSig",
		FetchedAt: time.Now(),
	}

	mockF := &mockFetcher{
		skins: map[string]*model.SkinData{
			"Notch": notchSkin,
		},
	}

	cfg := &ConfigSnapshot{
		Enabled:         true,
		AutoSkinOffline: true,
		Cooldown:        5 * time.Second,
	}

	mgr := NewManager(nil, mockF, store, func() *ConfigSnapshot { return cfg })

	// 1. Test Offline Auto-skin on login
	offlineUUID, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
	origProf := profile.GameProfile{
		ID:   offlineUUID,
		Name: "Notch",
	}

	processed := mgr.ProcessProfileRequest(context.Background(), origProf, false)
	if len(processed.Properties) == 0 {
		t.Fatalf("expected textures property injected for Notch")
	}
	if processed.Properties[0].Name != "textures" || processed.Properties[0].Value != "notchTexturesValue" {
		t.Fatalf("unexpected property: %v", processed.Properties[0])
	}

	// 2. Test User explicit skin override
	customSkin := &model.SkinData{
		Name:      "Herobrine",
		Value:     "herobrineTextures",
		Signature: "herobrineSig",
		FetchedAt: time.Now(),
	}
	_ = store.SetUserSkin(offlineUUID, "Notch", "Herobrine", customSkin)

	processed2 := mgr.ProcessProfileRequest(context.Background(), origProf, false)
	if processed2.Properties[0].Value != "herobrineTextures" {
		t.Fatalf("expected overridden skin, got: %s", processed2.Properties[0].Value)
	}

	// 3. Test Cooldown
	onCd, remaining := mgr.CheckCooldown(offlineUUID)
	if onCd {
		t.Fatalf("expected no cooldown before recording")
	}

	mgr.RecordCooldown(offlineUUID)
	onCd, remaining = mgr.CheckCooldown(offlineUUID)
	if !onCd || remaining <= 0 {
		t.Fatalf("expected active cooldown, got onCd=%v, rem=%v", onCd, remaining)
	}
}
