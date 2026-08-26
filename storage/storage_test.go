package storage

import (
	"os"
	"testing"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestStorageOperationsAndPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skins_storage_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStorage(tmpDir, 1*time.Hour)
	if err := store.Load(); err != nil {
		t.Fatalf("failed load: %v", err)
	}

	testUUID, _ := uuid.Parse("11111111-1111-1111-1111-111111111111")
	skin := &model.SkinData{
		Name:      "Notch",
		Value:     "val123",
		Signature: "sig123",
		FetchedAt: time.Now().UTC(),
	}

	// Set user skin
	if err := store.SetUserSkin(testUUID, "Player1", "Notch", skin); err != nil {
		t.Fatalf("failed set: %v", err)
	}

	// Cache skin
	store.SetCachedSkin("Notch", skin)
	if err := store.SaveCache(); err != nil {
		t.Fatalf("failed save cache: %v", err)
	}

	// Retrieve by UUID & Username
	u := store.GetUserSkin(testUUID)
	if u == nil || u.SkinName != "Notch" || u.Skin.Value != "val123" {
		t.Fatalf("expected user skin Notch, got %v", u)
	}

	u2 := store.GetUserSkinByUsername("player1")
	if u2 == nil || u2.PlayerUUID != testUUID {
		t.Fatalf("expected user by username, got %v", u2)
	}

	cached := store.GetCachedSkin("notch")
	if cached == nil || cached.Signature != "sig123" {
		t.Fatalf("expected cached skin, got %v", cached)
	}

	// Reload from disk
	store2 := NewStorage(tmpDir, 1*time.Hour)
	if err := store2.Load(); err != nil {
		t.Fatalf("failed load store2: %v", err)
	}

	uReloaded := store2.GetUserSkin(testUUID)
	if uReloaded == nil || uReloaded.SkinName != "Notch" {
		t.Fatalf("expected reloaded user skin, got %v", uReloaded)
	}

	cachedReloaded := store2.GetCachedSkin("Notch")
	if cachedReloaded == nil || cachedReloaded.Value != "val123" {
		t.Fatalf("expected reloaded cached skin, got %v", cachedReloaded)
	}

	// Remove
	removed, err := store2.RemoveUserSkin(testUUID)
	if err != nil || !removed {
		t.Fatalf("expected removed true, got %v (%v)", removed, err)
	}

	if store2.GetUserSkin(testUUID) != nil {
		t.Fatalf("expected user skin cleared")
	}
}
