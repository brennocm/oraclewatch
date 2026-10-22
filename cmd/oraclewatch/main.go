// Command oraclewatch monitors Chainlink Data Feeds from the consumer side and
// alerts on conditions that have historically led to losses in protocols that
// depend on them. It only ever reads from the chain; it never sends
// transactions and holds no private keys.
package main

import (
	"os"

	"github.com/brennocm/oraclewatch/internal/cli"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
