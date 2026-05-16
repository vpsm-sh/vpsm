package auth

import "testing"

func TestIsKnownProvider_Vultr(t *testing.T) {
	if !isKnownProvider("vultr") {
		t.Fatal("expected Vultr to be known")
	}
}
