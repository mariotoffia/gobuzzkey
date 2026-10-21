// Package live runs gobuzzkey against a real Buzz relay. It is skipped unless
// GOBUZZKEY_LIVE_ENV names an identity file and -short is off (TESTS.md §1).
package live

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mariotoffia/gobuzzkey/adapters/crypto/secp256k1"
	"github.com/mariotoffia/gobuzzkey/adapters/relay/buzzhttp"
	"github.com/mariotoffia/gobuzzkey/adapters/store/envfile"
	"github.com/mariotoffia/gobuzzkey/app"
	"github.com/mariotoffia/gobuzzkey/domain/clock"
)

// TestLiveRelay_AdmitsTheIdentity proves a stored identity is admitted by its
// relay and, when GOBUZZKEY_LIVE_CHANNEL is set, can read that channel.
// Nothing is posted.
func TestLiveRelay_AdmitsTheIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("live relay test skipped in -short mode")
	}
	path := os.Getenv("GOBUZZKEY_LIVE_ENV")
	if path == "" {
		t.Skip("set GOBUZZKEY_LIVE_ENV to an identity file to run against its relay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	crypto := secp256k1.New()
	store, err := envfile.New(path, envfile.FormatEnv)
	if err != nil {
		t.Fatal(err)
	}
	id, err := app.NewInspector(crypto, crypto, clock.Real{}).Open(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := app.NewChecker(crypto, clock.Real{}).Check(ctx, id, buzzhttp.New(id, crypto),
		app.CheckRequest{Channel: os.Getenv("GOBUZZKEY_LIVE_CHANNEL")})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("admitted; %d channel(s) visible", len(rep.Channels))
}
