// Package chain is the RPC layer: one Endpoint per configured RPC URL, with
// timeouts, retries, a per-endpoint circuit breaker and chain ID checks, and a
// Network that chooses the single block every read of a cycle is made at.
//
// The rest of the program depends only on the small Backend and Caller
// interfaces, which tests satisfy with fakes or the go-ethereum simulated
// backend.
package chain

import (
	"context"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// Backend is the subset of the Ethereum JSON-RPC API that oraclewatch uses:
// eth_chainId, eth_getBlockByNumber and eth_call. There is deliberately no
// method that sends a transaction. *ethclient.Client and the simulated
// backend's client satisfy it.
type Backend interface {
	ChainID(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// Caller performs eth_call at a given block. It is what the feeds package needs.
type Caller interface {
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// UserAgent identifies oraclewatch to RPC providers.
const UserAgent = "oraclewatch (+https://github.com/brennocm/oraclewatch)"

// maxIdleConnsPerHost keeps a small pool per endpoint; a cycle sends a handful
// of requests per endpoint.
const maxIdleConnsPerHost = 4

// DialHTTP returns a Backend for an HTTP(S) JSON-RPC URL. No connection is made
// until the first request. Per-call deadlines come from the context; the
// transport timeouts below bound the phases a context deadline cannot see
// (for example a stalled TLS handshake on a pooled dial).
func DialHTTP(ctx context.Context, rawURL string) (Backend, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		ForceAttemptHTTP2:     true,
	}
	client, err := rpc.DialOptions(ctx, rawURL,
		rpc.WithHTTPClient(&http.Client{Transport: transport}),
		rpc.WithHeader("User-Agent", UserAgent),
	)
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(client), nil
}
