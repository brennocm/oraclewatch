package cli

import (
	"math/big"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rogpeppe/go-internal/testscript"

	"github.com/brennocm/oraclewatch/internal/testutil/fakerpc"
)

// TestMain lets testscript run the real CLI as the "oraclewatch" command.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"oraclewatch": func() { os.Exit(Execute(BuildInfo{Version: "v0.0.0-test", Commit: "e2e", Date: "today"})) },
	})
}

const (
	e2eProxy    = "0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419"
	e2eAgg      = "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5"
	e2eSeqProxy = "0xFdB631F5EE196F0ed6FAa767959853A9F217697D"
)

func e2eChain(id uint64, updatedAgo time.Duration, sequencer bool) *fakerpc.Chain {
	now := time.Now()
	c := fakerpc.New(id, now)
	c.Feeds[common.HexToAddress(e2eProxy)] = &fakerpc.Feed{
		Decimals: 8, Description: "ETH / USD", Aggregator: common.HexToAddress(e2eAgg), Phase: 7,
		Round: big.NewInt(1000), Answer: big.NewInt(244363658508),
		StartedAt: now.Add(-updatedAgo).Unix() - 10, UpdatedAt: now.Add(-updatedAgo).Unix(),
		Min: big.NewInt(1), Max: new(big.Int).Lsh(big.NewInt(1), 180), TypeAndVersion: "AccessControlledOCR2Aggregator 1.0.0",
	}
	if sequencer {
		c.Feeds[common.HexToAddress(e2eSeqProxy)] = &fakerpc.Feed{
			Decimals: 0, Description: "L2 Sequencer Uptime Status Feed", Aggregator: common.HexToAddress("0xC1303BBBaf172C55848D3Cb91606d8E27FF38428"),
			Phase: 1, Round: big.NewInt(41), Answer: big.NewInt(0),
			StartedAt: now.Add(-48 * time.Hour).Unix(), UpdatedAt: now.Add(-48 * time.Hour).Unix(),
		}
	}
	return c
}

func TestScripts(t *testing.T) {
	// testscript runs scripts as parallel subtests that outlive this
	// function body, so the servers are closed with t.Cleanup, not defer.
	healthy := httptest.NewServer(e2eChain(1, time.Minute, false).Handler())
	t.Cleanup(healthy.Close)
	stale := httptest.NewServer(e2eChain(1, 3*time.Hour, false).Handler())
	t.Cleanup(stale.Close)
	l2 := httptest.NewServer(e2eChain(42161, time.Minute, true).Handler())
	t.Cleanup(l2.Close)

	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("ETH_RPC", healthy.URL)
			env.Setenv("STALE_RPC", stale.URL)
			env.Setenv("ARB_RPC", l2.URL)
			// A URL carrying a fake API key on a closed port: used to prove
			// that keys never reach the output, even in error messages.
			env.Setenv("SECRET_RPC", "http://127.0.0.1:1/v2/sk_live_e2e_secret_0123456789")
			return nil
		},
	})
}
