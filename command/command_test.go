package command

import (
	"strings"
	"testing"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/brigodier"
	c "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/util/permission"
)

type mockSource struct {
	messages []string
	perms    map[string]bool
}

func (m *mockSource) SendMessage(msg c.Component, opts ...command.MessageOption) error {
	if t, ok := msg.(*c.Text); ok {
		m.messages = append(m.messages, t.Content)
	}
	return nil
}

func (m *mockSource) HasPermission(p string) bool {
	return m.perms[p] || m.perms["*"]
}

func (m *mockSource) PermissionValue(p string) permission.TriState {
	if m.HasPermission(p) {
		return permission.True
	}
	return permission.False
}

func TestSendHelp(t *testing.T) {
	src := &mockSource{perms: map[string]bool{"skinschanger.use": true}}
	if err := sendHelp(src, "skin"); err != nil {
		t.Fatal(err)
	}
	if len(src.messages) == 0 || !strings.Contains(src.messages[0], "SkinsChanger Commands") {
		t.Fatalf("unexpected help message: %v", src.messages)
	}
}

func TestSuggestionsBuilder(t *testing.T) {
	candidates := []string{"Notch", "Dinnerbone", "Technoblade"}
	suggester := suggestCandidates(func() []string { return candidates })

	builder := &brigodier.SuggestionsBuilder{
		Input:          "skin set No",
		InputLowerCase: "skin set no",
		Start:          9,
	}

	res := suggester(nil, builder)
	if len(res.Suggestions) != 1 || res.Suggestions[0].Text != "Notch" {
		t.Fatalf("expected suggestion 'Notch', got: %v", res.Suggestions)
	}
	if res.Suggestions[0].Range.Start != 9 || res.Suggestions[0].Range.End != 11 {
		t.Fatalf("expected range [9, 11], got [%d, %d]", res.Suggestions[0].Range.Start, res.Suggestions[0].Range.End)
	}
}

func TestModelConversion(t *testing.T) {
	if string(model.ModelClassic) != "classic" || string(model.ModelSlim) != "slim" {
		t.Fatalf("unexpected model strings")
	}
}
