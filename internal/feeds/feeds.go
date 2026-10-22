// Package feeds reads Chainlink Data Feed contracts through eth_call at an
// explicit block: latest round, historical rounds, metadata (decimals,
// description, version, current aggregator, phase) and the aggregator's
// minAnswer/maxAnswer bounds.
//
// Every function takes the block number to read at. Callers pass the pinned
// block of the cycle so that all reads are mutually consistent.
package feeds

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/brennocm/oraclewatch/internal/chain"
)

// ErrEmptyResult means eth_call returned no data: there is no contract at the
// address at that block, or the function does not exist and the contract has
// no fallback. It is reported instead of an opaque ABI decoding error.
var ErrEmptyResult = errors.New("eth_call returned no data (no contract at this address or unknown function)")

// phaseOffset is the bit position of the phase ID in a proxy round ID:
// roundId = (phaseId << 64) | aggregatorRoundId.
const phaseOffset = 64

// Round is the raw output of latestRoundData/getRoundData.
type Round struct {
	RoundID         *big.Int
	Answer          *big.Int
	StartedAt       *big.Int
	UpdatedAt       *big.Int
	AnsweredInRound *big.Int // deprecated by Chainlink; kept only for display
}

// PhaseID returns the phase encoded in a proxy round ID.
func (r Round) PhaseID() uint16 {
	return uint16(new(big.Int).Rsh(r.RoundID, phaseOffset).Uint64()) //nolint:gosec // G115: a uint80 round ID shifted right by 64 has at most 16 bits
}

// AggregatorRoundID returns the aggregator's own round ID (the low 64 bits).
func (r Round) AggregatorRoundID() uint64 {
	mask := new(big.Int).SetUint64(^uint64(0))
	return new(big.Int).And(r.RoundID, mask).Uint64()
}

// Equal reports whether two rounds are identical in every field. It is the
// comparison behind FW-RPC-001: two honest nodes at the same block hash must
// return exactly the same tuple.
func (r Round) Equal(o Round) bool {
	return cmpEq(r.RoundID, o.RoundID) && cmpEq(r.Answer, o.Answer) && cmpEq(r.StartedAt, o.StartedAt) &&
		cmpEq(r.UpdatedAt, o.UpdatedAt) && cmpEq(r.AnsweredInRound, o.AnsweredInRound)
}

func cmpEq(a, b *big.Int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Cmp(b) == 0
}

// TimeOf converts a uint256 timestamp to time.Time. Values that do not fit in
// int64 seconds are clamped; such values are themselves a finding for
// FW-INVALID-001 and are compared as *big.Int there.
func TimeOf(ts *big.Int) time.Time {
	if ts == nil || ts.Sign() <= 0 {
		return time.Unix(0, 0).UTC()
	}
	if !ts.IsInt64() {
		return time.Unix(1<<62, 0).UTC()
	}
	return time.Unix(ts.Int64(), 0).UTC()
}

// Reading is one feed read at one block on one endpoint.
type Reading struct {
	Proxy     common.Address
	Round     Round
	Decimals  uint8
	Price     *big.Rat // Answer / 10^Decimals
	Block     uint64
	BlockTime time.Time
	Endpoint  string
}

// Normalize returns answer / 10^decimals as an exact rational.
func Normalize(answer *big.Int, decimals uint8) *big.Rat {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Rat).SetFrac(answer, scale)
}

// call sends one eth_call and decodes its result.
func call[T any](ctx context.Context, c chain.Caller, to common.Address, block *big.Int, data []byte, unpack func([]byte) (T, error)) (T, error) {
	var zero T
	out, err := c.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, block)
	if err != nil {
		return zero, err
	}
	if len(out) == 0 {
		return zero, ErrEmptyResult
	}
	v, err := unpack(out)
	if err != nil {
		return zero, fmt.Errorf("decode: %w", err)
	}
	return v, nil
}

// LatestRound calls latestRoundData() on proxy at block.
func LatestRound(ctx context.Context, c chain.Caller, proxy common.Address, block *big.Int) (Round, error) {
	r, err := call(ctx, c, proxy, block, pack(aggregatorV3ABI, "latestRoundData"), func(b []byte) (Round, error) {
		return unpackRound("latestRoundData", b)
	})
	if err != nil {
		return Round{}, fmt.Errorf("latestRoundData: %w", err)
	}
	return r, nil
}

// RoundData calls getRoundData(roundID) on proxy at block. On OCR2
// aggregators an unknown round returns zeros instead of reverting
// (docs/research.md §1.2), so callers must check UpdatedAt.
func RoundData(ctx context.Context, c chain.Caller, proxy common.Address, block, roundID *big.Int) (Round, error) {
	r, err := call(ctx, c, proxy, block, pack(aggregatorV3ABI, "getRoundData", roundID), func(b []byte) (Round, error) {
		return unpackRound("getRoundData", b)
	})
	if err != nil {
		return Round{}, fmt.Errorf("getRoundData(%s): %w", roundID, err)
	}
	return r, nil
}

// ReadLatest reads the latest round of a feed and normalises it with decimals
// (taken from the metadata read for this feed).
func ReadLatest(ctx context.Context, c chain.Caller, endpoint string, proxy common.Address, decimals uint8, block uint64, blockTime time.Time) (Reading, error) {
	r, err := LatestRound(ctx, c, proxy, new(big.Int).SetUint64(block))
	if err != nil {
		return Reading{}, err
	}
	return Reading{
		Proxy: proxy, Round: r, Decimals: decimals, Price: Normalize(r.Answer, decimals),
		Block: block, BlockTime: blockTime, Endpoint: endpoint,
	}, nil
}

// Decimals calls decimals() on proxy at block.
func Decimals(ctx context.Context, c chain.Caller, proxy common.Address, block *big.Int) (uint8, error) {
	d, err := call(ctx, c, proxy, block, pack(aggregatorV3ABI, "decimals"), func(b []byte) (uint8, error) {
		return unpackOne[uint8](aggregatorV3ABI, "decimals", b)
	})
	if err != nil {
		return 0, fmt.Errorf("decimals: %w", err)
	}
	return d, nil
}

// Metadata describes a proxy and the aggregator it currently points to.
type Metadata struct {
	Decimals    uint8
	Description string
	Version     *big.Int
	Aggregator  common.Address
	PhaseID     uint16
}

// ReadMetadata reads decimals, description, version, aggregator and phaseId.
func ReadMetadata(ctx context.Context, c chain.Caller, proxy common.Address, block *big.Int) (Metadata, error) {
	var m Metadata
	var err error
	if m.Decimals, err = call(ctx, c, proxy, block, pack(aggregatorV3ABI, "decimals"), func(b []byte) (uint8, error) {
		return unpackOne[uint8](aggregatorV3ABI, "decimals", b)
	}); err != nil {
		return m, fmt.Errorf("decimals: %w", err)
	}
	if m.Description, err = call(ctx, c, proxy, block, pack(aggregatorV3ABI, "description"), func(b []byte) (string, error) {
		d, err := unpackOne[string](aggregatorV3ABI, "description", b)
		return SanitizeText(d), err
	}); err != nil {
		return m, fmt.Errorf("description: %w", err)
	}
	if m.Version, err = call(ctx, c, proxy, block, pack(aggregatorV3ABI, "version"), func(b []byte) (*big.Int, error) {
		return unpackOne[*big.Int](aggregatorV3ABI, "version", b)
	}); err != nil {
		return m, fmt.Errorf("version: %w", err)
	}
	if m.Aggregator, err = call(ctx, c, proxy, block, pack(proxyABI, "aggregator"), func(b []byte) (common.Address, error) {
		return unpackAddress(proxyABI, "aggregator", b)
	}); err != nil {
		return m, fmt.Errorf("aggregator: %w", err)
	}
	if m.PhaseID, err = call(ctx, c, proxy, block, pack(proxyABI, "phaseId"), func(b []byte) (uint16, error) {
		return unpackOne[uint16](proxyABI, "phaseId", b)
	}); err != nil {
		return m, fmt.Errorf("phaseId: %w", err)
	}
	return m, nil
}

// Bounds are the aggregator's minAnswer/maxAnswer and its typeAndVersion.
type Bounds struct {
	Min, Max *big.Int
	// TypeAndVersion is empty if the aggregator does not implement it; the
	// circuit-breaker check then treats the bounds' behaviour as unverified.
	TypeAndVersion string
}

// ReadBounds reads minAnswer, maxAnswer and typeAndVersion from an aggregator
// (not the proxy: the proxy does not forward them).
func ReadBounds(ctx context.Context, c chain.Caller, aggregator common.Address, block *big.Int) (Bounds, error) {
	var b Bounds
	var err error
	bigInt := func(method string) func([]byte) (*big.Int, error) {
		return func(d []byte) (*big.Int, error) { return unpackOne[*big.Int](boundsABI, method, d) }
	}
	if b.Min, err = call(ctx, c, aggregator, block, pack(boundsABI, "minAnswer"), bigInt("minAnswer")); err != nil {
		return b, fmt.Errorf("minAnswer: %w", err)
	}
	if b.Max, err = call(ctx, c, aggregator, block, pack(boundsABI, "maxAnswer"), bigInt("maxAnswer")); err != nil {
		return b, fmt.Errorf("maxAnswer: %w", err)
	}
	tv, err := call(ctx, c, aggregator, block, pack(boundsABI, "typeAndVersion"), func(d []byte) (string, error) {
		return unpackOne[string](boundsABI, "typeAndVersion", d)
	})
	if err == nil {
		b.TypeAndVersion = SanitizeText(tv)
	}
	return b, nil
}

// maxTextLen bounds strings read from contracts.
const maxTextLen = 128

// SanitizeText makes a string read from a contract safe to log and to put in
// alerts. Contract strings (and anything a lying RPC returns) are untrusted:
// they could carry terminal escape sequences or be huge. Only printable ASCII
// is kept (other bytes become '?') and the length is capped. Feed
// descriptions such as "ETH / USD" are unaffected, and a sanitised value
// still differs from the configured one, so FW-META-001 still fires.
func SanitizeText(s string) string {
	b := []byte(s)
	if len(b) > maxTextLen {
		b = b[:maxTextLen]
	}
	for i, c := range b {
		if c < 0x20 || c > 0x7e {
			b[i] = '?'
		}
	}
	return string(b)
}

// Change is one metadata field that differs between two reads.
type Change struct {
	Field    string // "decimals", "description", "version", "aggregator", "phase_id"
	Old, New string
}

// Diff lists the fields that differ between two metadata reads of a proxy.
func Diff(prev, cur Metadata) []Change {
	var out []Change
	add := func(field, a, b string) {
		if a != b {
			out = append(out, Change{Field: field, Old: a, New: b})
		}
	}
	add("decimals", fmt.Sprint(prev.Decimals), fmt.Sprint(cur.Decimals))
	add("description", prev.Description, cur.Description)
	add("version", fmt.Sprint(prev.Version), fmt.Sprint(cur.Version))
	add("aggregator", prev.Aggregator.Hex(), cur.Aggregator.Hex())
	add("phase_id", fmt.Sprint(prev.PhaseID), fmt.Sprint(cur.PhaseID))
	return out
}
