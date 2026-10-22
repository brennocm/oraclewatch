package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/brennocm/oraclewatch/internal/clock"
)

// Network groups the endpoints of one chain.
type Network struct {
	Name          string
	ChainID       uint64
	Confirmations uint64
	MaxHeadLag    time.Duration
	Endpoints     []*Endpoint
	Clock         clock.Clock
	// RequireVerified excludes endpoints whose eth_chainId has not matched
	// yet. Set for every network built from configuration, so an endpoint
	// whose first chain ID check failed transiently is never read unverified.
	RequireVerified bool
}

// Head is what one endpoint reported as its latest block.
type Head struct {
	Endpoint string
	Number   uint64
	Time     time.Time
	Err      error // the eth_getBlockByNumber error, if any
	Lagging  bool  // head older than MaxHeadLag
}

// Snapshot is the outcome of choosing the block that every read of a cycle is
// made at.
//
// Reading every feed at one fixed block, on every endpoint, is what makes the
// results comparable: values from different blocks can legitimately differ,
// values from the same block (with the same hash) cannot.
type Snapshot struct {
	Network   string
	Taken     time.Time
	Heads     []Head
	Block     uint64      // pinned block number
	BlockHash common.Hash // hash agreed by Readers
	BlockTime time.Time   // timestamp of the pinned block
	// Readers are the endpoints that returned the agreed hash for Block; reads
	// of the cycle use them. The first one is the preferred endpoint.
	Readers []*Endpoint
	// Forked lists endpoints that returned a different hash for Block
	// (FW-RPC-002): they are excluded from the cycle's reads.
	Forked []ForkedEndpoint
}

// ForkedEndpoint is an endpoint whose view of the pinned block differs.
type ForkedEndpoint struct {
	Endpoint string
	Hash     common.Hash
}

// BlockNumber returns Block as a *big.Int for eth_call.
func (s *Snapshot) BlockNumber() *big.Int { return new(big.Int).SetUint64(s.Block) }

// AvailableEndpoints returns the endpoints that may be used now.
func (n *Network) AvailableEndpoints() []*Endpoint {
	var out []*Endpoint
	for _, e := range n.Endpoints {
		if e.Available() && (!n.RequireVerified || e.Verified()) {
			out = append(out, e)
		}
	}
	return out
}

// HasUnverified reports whether an enabled endpoint still needs a successful
// chain ID check.
func (n *Network) HasUnverified() bool {
	for _, e := range n.Endpoints {
		if e.Disabled() == nil && !e.Verified() {
			return true
		}
	}
	return false
}

// VerifyChainIDs checks eth_chainId on every enabled endpoint. Endpoints that
// serve another chain are disabled; their errors are returned so that the
// caller can raise FW-RPC-003. Endpoints that could not be checked stay
// enabled and appear in the returned map with their call error.
func (n *Network) VerifyChainIDs(ctx context.Context) map[string]error {
	results := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range n.Endpoints {
		if e.Disabled() != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.VerifyChainID(ctx, n.ChainID); err != nil {
				mu.Lock()
				results[e.Name()] = err
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}

// Snapshot asks every available endpoint for its head, discards lagging
// ones, pins Block = min(head) - Confirmations, and fetches the pinned header
// from each remaining endpoint to compare hashes.
//
// It returns ErrChainStalled when every endpoint answered with an old head,
// and ErrNoHealthyEndpoint when no endpoint could be used. The partially
// filled Snapshot is returned with those errors so that the caller can report
// per-endpoint heads.
func (n *Network) Snapshot(ctx context.Context) (*Snapshot, error) {
	now := n.Clock.Now()
	snap := &Snapshot{Network: n.Name, Taken: now}
	endpoints := n.AvailableEndpoints()
	if len(endpoints) == 0 {
		return snap, ErrNoHealthyEndpoint
	}

	// 1. Heads, concurrently. Each goroutine writes only its own slot.
	heads := make([]Head, len(endpoints))
	var wg sync.WaitGroup
	for i, e := range endpoints {
		wg.Add(1)
		go func() {
			defer wg.Done()
			heads[i] = Head{Endpoint: e.Name()}
			h, err := e.HeaderByNumber(ctx, nil)
			if err != nil {
				heads[i].Err = err
				return
			}
			heads[i].Number = h.Number.Uint64()
			heads[i].Time = time.Unix(int64(h.Time), 0) //nolint:gosec // G115: block timestamps fit in int64 until year 292277026596
			heads[i].Lagging = now.Sub(heads[i].Time) > n.MaxHeadLag
		}()
	}
	wg.Wait()
	snap.Heads = heads

	var candidates []*Endpoint
	var minHead uint64
	answered := 0
	for i, h := range heads {
		if h.Err != nil {
			continue
		}
		answered++
		if h.Lagging {
			continue
		}
		if len(candidates) == 0 || h.Number < minHead {
			minHead = h.Number
		}
		candidates = append(candidates, endpoints[i])
	}
	if len(candidates) == 0 {
		if answered > 0 {
			return snap, ErrChainStalled
		}
		return snap, ErrNoHealthyEndpoint
	}
	if minHead < n.Confirmations {
		return snap, fmt.Errorf("head %d is below the %d confirmations", minHead, n.Confirmations)
	}
	snap.Block = minHead - n.Confirmations

	// 2. Pinned header from each candidate.
	type pinned struct {
		hash common.Hash
		time time.Time
		err  error
	}
	results := make([]pinned, len(candidates))
	for i, e := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := e.HeaderByNumber(ctx, snap.BlockNumber())
			if err != nil {
				results[i].err = err
				return
			}
			results[i] = pinned{hash: h.Hash(), time: time.Unix(int64(h.Time), 0)} //nolint:gosec // G115: see above
		}()
	}
	wg.Wait()

	// 3. Group by hash. The largest group wins; ties go to the endpoint
	// listed first in the configuration, so the choice is deterministic.
	count := map[common.Hash]int{}
	var order []common.Hash
	for _, r := range results {
		if r.err != nil {
			continue
		}
		if count[r.hash] == 0 {
			order = append(order, r.hash)
		}
		count[r.hash]++
	}
	if len(order) == 0 {
		return snap, errors.Join(ErrNoHealthyEndpoint, results[0].err)
	}
	best := order[0]
	for _, h := range order[1:] {
		if count[h] > count[best] {
			best = h
		}
	}
	snap.BlockHash = best
	for i, r := range results {
		switch {
		case r.err != nil:
			// Already counted by the endpoint's breaker; not a fork.
		case r.hash == best:
			snap.Readers = append(snap.Readers, candidates[i])
			snap.BlockTime = r.time
		default:
			snap.Forked = append(snap.Forked, ForkedEndpoint{Endpoint: candidates[i].Name(), Hash: r.hash})
		}
	}
	return snap, nil
}
