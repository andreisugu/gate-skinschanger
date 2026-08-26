package model

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

// SkinModel defines the arm thickness model ("classic" 4px or "slim" 3px Alex style).
type SkinModel string

const (
	ModelClassic SkinModel = "classic"
	ModelSlim    SkinModel = "slim"
	ModelDefault SkinModel = ""
)

// SkinData contains the signed textures property and metadata for a Minecraft skin.
type SkinData struct {
	Name      string    `json:"name"`                // Account/Skin identifier (e.g. "Notch" or "custom_url")
	Value     string    `json:"value"`               // Base64-encoded textures JSON
	Signature string    `json:"signature,omitempty"` // Cryptographic Mojang RSA signature
	Model     SkinModel `json:"model,omitempty"`     // Model variant (classic or slim)
	SkinURL   string    `json:"skin_url,omitempty"`  // Direct PNG URL (textures.minecraft.net/...)
	FetchedAt time.Time `json:"fetched_at"`          // Timestamp when skin was fetched
}

// ToProperty converts SkinData to a Gate profile.Property for profile injection.
func (s *SkinData) ToProperty() profile.Property {
	return profile.Property{
		Name:      "textures",
		Value:     s.Value,
		Signature: s.Signature,
	}
}

// TexturePayload represents the decoded JSON object within the Base64 Value.
type TexturePayload struct {
	Timestamp   int64  `json:"timestamp"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	Textures    struct {
		Skin struct {
			URL      string `json:"url"`
			Metadata struct {
				Model string `json:"model,omitempty"`
			} `json:"metadata,omitempty"`
		} `json:"SKIN,omitempty"`
		Cape struct {
			URL string `json:"url"`
		} `json:"CAPE,omitempty"`
	} `json:"textures"`
}

// ExtractMetadata extracts direct SkinURL and SkinModel from the base64 value.
func (s *SkinData) ExtractMetadata() {
	if s.Value == "" {
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(s.Value)
	if err != nil {
		return
	}
	var payload TexturePayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return
	}
	if payload.Textures.Skin.URL != "" {
		s.SkinURL = payload.Textures.Skin.URL
	}
	if payload.Textures.Skin.Metadata.Model == "slim" {
		s.Model = ModelSlim
	} else if payload.Textures.Skin.URL != "" {
		s.Model = ModelClassic
	}
}

// UserSkin represents a player's assigned skin mapping.
type UserSkin struct {
	PlayerUUID uuid.UUID `json:"player_uuid"`
	Username   string    `json:"username"`
	SkinName   string    `json:"skin_name"` // The skin target name or "url:..."
	Skin       *SkinData `json:"skin"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NormalizeName canonicalizes a username or skin identifier.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
