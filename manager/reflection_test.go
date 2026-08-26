package manager

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

type dummyPlayer struct {
	profile *profile.GameProfile
}

func (d *dummyPlayer) ID() uuid.UUID {
	return d.profile.ID
}

func (d *dummyPlayer) Username() string {
	return d.profile.Name
}

func TestInjectPlayerProfileProperty(t *testing.T) {
	origUUID, _ := uuid.Parse("069a79f4-44e9-4726-a5be-fca90e38aaf5")
	prof := &profile.GameProfile{
		ID:   origUUID,
		Name: "TestUser",
		Properties: []profile.Property{
			{Name: "textures", Value: "oldVal", Signature: "oldSig"},
		},
	}

	player := &dummyPlayer{profile: prof}

	newProp := profile.Property{
		Name:      "textures",
		Value:     "newVal123",
		Signature: "newSig123",
	}

	success := InjectPlayerProfileProperty(any(player).(interface{}), newProp)
	if !success {
		t.Fatalf("expected injection to succeed")
	}

	if len(prof.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(prof.Properties))
	}

	if prof.Properties[0].Value != "newVal123" || prof.Properties[0].Signature != "newSig123" {
		t.Fatalf("unexpected property: %v", prof.Properties[0])
	}
}
