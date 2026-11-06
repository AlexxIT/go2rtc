package jooan

import (
	"strings"
	"testing"
)

func TestHandleAcceptsOnlyAliasAndDoesNotExposeSecrets(t *testing.T) {
	aliases = map[string]AliasConfig{
		"patio": {Host: "192.0.2.10", UID: "synthetic-uid", Username: "admin", Password: "synthetic-secret"},
	}
	for _, source := range []string{"jooan:patio?password=leak", "jooan:patio@example", "jooan:unknown"} {
		_, err := handle(source)
		if err == nil {
			t.Fatalf("handle(%q) unexpectedly succeeded", source)
		}
		if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "synthetic-uid") {
			t.Fatalf("error disclosed secret: %v", err)
		}
	}
}
