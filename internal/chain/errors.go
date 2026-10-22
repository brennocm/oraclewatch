package chain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/ethereum/go-ethereum/rpc"
)

// Sentinel errors returned by Endpoint and Network.
var (
	// ErrCircuitOpen means the endpoint failed repeatedly and is resting.
	ErrCircuitOpen = errors.New("endpoint circuit breaker is open")
	// ErrNoHealthyEndpoint means no endpoint of the network could be used.
	ErrNoHealthyEndpoint = errors.New("no healthy RPC endpoint")
	// ErrChainStalled means every endpoint answered but all heads are older
	// than max_head_lag: the chain (or every provider) has stopped.
	ErrChainStalled = errors.New("every endpoint reports a stale head")
)

// WrongChainError disables an endpoint whose eth_chainId differs from the
// configured chain ID.
type WrongChainError struct {
	Want, Got uint64
}

func (e *WrongChainError) Error() string {
	return fmt.Sprintf("endpoint serves chain %d, configured chain is %d", e.Got, e.Want)
}

// EndpointError attributes a failure to an endpoint by name. Err has already
// been redacted.
type EndpointError struct {
	Endpoint string
	Method   string
	Err      error
}

func (e *EndpointError) Error() string {
	return fmt.Sprintf("endpoint %s: %s: %v", e.Endpoint, e.Method, e.Err)
}

func (e *EndpointError) Unwrap() error { return e.Err }

// jsonRPCLimitExceeded is the JSON-RPC error code providers use for rate
// limiting (EIP-1474 "limit exceeded").
const jsonRPCLimitExceeded = -32005

// IsTransient reports whether err is worth retrying on the same endpoint:
// timeouts, connection failures, HTTP 429 and 5xx, and JSON-RPC rate limits.
// A JSON-RPC error such as "execution reverted" is an answer, not a failure,
// and is not retried.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var httpErr rpc.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= http.StatusInternalServerError
	}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		return rpcErr.ErrorCode() == jsonRPCLimitExceeded
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// isJSONRPCAnswer reports whether err is an error object returned by the
// JSON-RPC server, as opposed to a transport or HTTP failure.
func isJSONRPCAnswer(err error) bool {
	var rpcErr rpc.Error
	var httpErr rpc.HTTPError
	return errors.As(err, &rpcErr) && !errors.As(err, &httpErr)
}
