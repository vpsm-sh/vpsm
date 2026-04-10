// Package providers holds cross-domain provider metadata shared between
// resource domains and the auth subsystem.
package providers

import "nathanbeddoewebdev/vpsm/internal/util"

// CredentialKey describes a single credential field for a provider.
type CredentialKey struct {
	// Key is the suffix appended to the provider name to form the keychain key.
	// For single-token providers this is empty (key stored as just "<provider>").
	// For multi-credential providers this is set (e.g. "apikey", "secretapikey").
	Key string

	// Prompt is the human-readable label shown when prompting the user.
	Prompt string

	// Secret controls whether the input should be masked (e.g. passwords/tokens).
	Secret bool
}

// CredentialSpec describes the complete credential scheme for a provider.
type CredentialSpec struct {
	// Provider is the normalized provider name (e.g. "hetzner", "porkbun").
	Provider string

	// DisplayName is the human-readable provider name (e.g. "Hetzner", "Porkbun").
	DisplayName string

	// Keys lists each credential that must be stored.
	// Single-token providers have one entry with an empty Key.
	// Multi-credential providers have one entry per credential.
	Keys []CredentialKey
}

// KeychainKey returns the keychain key for the given CredentialKey.
// For single-token providers (empty Key suffix), it returns the provider name.
// For multi-credential providers it returns "<provider>-<key>".
func (s CredentialSpec) KeychainKey(k CredentialKey) string {
	if k.Key == "" {
		return s.Provider
	}
	return s.Provider + "-" + k.Key
}

// knownSpecs is the authoritative list of registered provider credential specs.
// The auth login command iterates this to know how to prompt for credentials.
var knownSpecs = []CredentialSpec{
	{
		Provider:    "hetzner",
		DisplayName: "Hetzner",
		Keys: []CredentialKey{
			{Key: "", Prompt: "API Token", Secret: true},
		},
	},
	{
		Provider:    "porkbun",
		DisplayName: "Porkbun",
		Keys: []CredentialKey{
			{Key: "apikey", Prompt: "API Key", Secret: true},
			{Key: "secretapikey", Prompt: "Secret API Key", Secret: true},
		},
	},
	{
		Provider:    "cloudflare",
		DisplayName: "Cloudflare",
		Keys: []CredentialKey{
			{Key: "", Prompt: "Account API Token (not Global API Key)", Secret: true},
		},
	},
}

// Lookup returns the CredentialSpec for the given provider name,
// or nil if no spec is registered for that provider.
func Lookup(providerName string) *CredentialSpec {
	normalized := util.NormalizeKey(providerName)
	for i := range knownSpecs {
		if knownSpecs[i].Provider == normalized {
			return &knownSpecs[i]
		}
	}
	return nil
}

// All returns a copy of all registered credential specs.
func All() []CredentialSpec {
	out := make([]CredentialSpec, len(knownSpecs))
	copy(out, knownSpecs)
	return out
}
