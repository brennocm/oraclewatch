package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestBinaryHasNoSigningCode enforces the read-only design at the dependency
// level: the oraclewatch binary must not link key management, transaction
// signing helpers or the test doubles.
func TestBinaryHasNoSigningCode(t *testing.T) {
	out, err := exec.CommandContext(context.Background(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	forbidden := []string{
		"github.com/ethereum/go-ethereum/accounts/keystore",
		"github.com/ethereum/go-ethereum/accounts/external",
		"github.com/ethereum/go-ethereum/accounts/usbwallet",
		"github.com/ethereum/go-ethereum/accounts/abi/bind",
		"github.com/ethereum/go-ethereum/ethclient/simulated",
		"github.com/brennocm/oraclewatch/internal/feeds/feedstest",
		"github.com/brennocm/oraclewatch/internal/testutil",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("binary depends on %s", dep)
			}
		}
	}
}
