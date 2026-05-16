package providers

import "testing"

func TestLookup_Vultr(t *testing.T) {
	spec := Lookup("vultr")
	if spec == nil {
		t.Fatal("expected Vultr credential spec")
	}

	if spec.Provider != "vultr" {
		t.Fatalf("Provider = %q, want %q", spec.Provider, "vultr")
	}
	if spec.DisplayName != "Vultr" {
		t.Fatalf("DisplayName = %q, want %q", spec.DisplayName, "Vultr")
	}
	if len(spec.Keys) != 1 {
		t.Fatalf("len(Keys) = %d, want 1", len(spec.Keys))
	}
	if spec.Keys[0].Key != "" {
		t.Fatalf("Keys[0].Key = %q, want empty single-token key", spec.Keys[0].Key)
	}
	if !spec.Keys[0].Secret {
		t.Fatal("expected Vultr token to be secret")
	}
}
