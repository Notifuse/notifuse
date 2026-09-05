package testutil

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/license"
)

// The integration suite runs LICENSED: an enterprise grant carrying every feature and
// no workspace ceiling. The flows it exercises — a dozen workspaces created in one
// process, sign-in through SSO, per-member permissions — are the ones a paying
// deployment runs, and on the free tier the fourth workspace answers 402 and the SSO
// button never renders. Community-tier refusals are asserted by the unit tests of each
// gate; nothing in this suite asserts one.
//
// The grant is minted here with the committed dev signing key
// (pkg/license/testdata/dev_signing_key.json), which only a binary built with
// -tags licdev trusts. Under any other build the key is refused, the deployment
// silently degrades to the free tier, and seven suites fail on a 402 nobody asked
// for — which is exactly how this harness first met the licence gates. So the key is
// verified against the compiled trust set before the server comes up, and Start
// refuses with the tag to add rather than let the suite run degraded.

var devLicense struct {
	once sync.Once
	key  string
	err  error
}

// DevLicenseKey returns the licence every test server runs under, minted once per
// process. The error is non-nil when the fixture cannot be read or when this binary does
// not trust the dev signing key, i.e. it was built without -tags licdev.
func DevLicenseKey() (string, error) {
	devLicense.once.Do(func() {
		devLicense.key, devLicense.err = mintDevLicense()
	})
	return devLicense.key, devLicense.err
}

func mintDevLicense() (string, error) {
	priv, err := loadDevSigningKey()
	if err != nil {
		return "", err
	}

	now := time.Now()
	raw, err := license.Mint(priv, &license.Claims{
		V:    license.SchemaVersion,
		LID:  "lic_integration_suite",
		Org:  "Notifuse integration suite",
		Sub:  "test@example.com",
		Tier: "enterprise",
		// Every feature this build knows how to gate. A feature added to domain without a
		// line here stays unlicensed in the suite, and its gate answers 402 the first time
		// an integration test reaches it — the same shape as the failure that produced
		// this file, so the list is worth keeping complete.
		Feat: []string{
			string(domain.FeatureRBAC),
			string(domain.FeatureSESTenant),
			string(domain.FeatureSSO),
			string(domain.FeatureAuditLogs),
			string(domain.FeatureTemplateI18n),
		},
		MaxWS: domain.UnlimitedWorkspaces,
		// Issued an hour ago so a runner whose clock runs ahead of the minting host is
		// still inside MaxClockSkew; a year of validity so the suite never trips the
		// grace-period boundary mid-run.
		IAT: now.Add(-time.Hour).Unix(),
		Exp: now.Add(365 * 24 * time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("mint integration licence: %w", err)
	}

	// The same check the licence service will make at startup, surfaced here where it can
	// stop the run instead of degrading it.
	if _, err := license.Parse(raw); err != nil {
		return "", fmt.Errorf("this binary does not trust the dev licence signing key (%w); "+
			"the integration suite must be built with -tags integration,licdev — see the "+
			"test-integration target in the Makefile", err)
	}
	return raw, nil
}

// loadDevSigningKey reads the committed dev key pair. The path is derived from this
// source file rather than the working directory, so it resolves the same way whether the
// test binary runs from tests/integration, the repository root, or a temporary directory.
func loadDevSigningKey() (ed25519.PrivateKey, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("cannot locate the testutil package on disk")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "pkg", "license", "testdata", "dev_signing_key.json")

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dev signing key fixture: %w", err)
	}

	var fixture struct {
		Private string `json:"private"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		return nil, fmt.Errorf("parse dev signing key fixture: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(fixture.Private)
	if err != nil {
		return nil, fmt.Errorf("decode dev signing key: %w", err)
	}
	if len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("dev signing key is %d bytes, want %d", len(decoded), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(decoded), nil
}
