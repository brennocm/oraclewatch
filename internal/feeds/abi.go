package feeds

import (
	"embed"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// The ABIs are embedded and decoded with accounts/abi directly. Generated
// bind/v2 bindings were tried first, but that package links go-ethereum's
// keystore and external signer; see ADR 0004.
//
//go:embed abi/*.abi.json
var abiFiles embed.FS

var (
	aggregatorV3ABI = mustABI("abi/AggregatorV3Interface.abi.json")
	proxyABI        = mustABI("abi/AggregatorProxy.abi.json")
	boundsABI       = mustABI("abi/AggregatorBounds.abi.json")
)

func mustABI(path string) abi.ABI {
	f, err := abiFiles.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close() //nolint:errcheck // embedded file
	a, err := abi.JSON(f)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", path, err))
	}
	return a
}

// pack encodes a call. The method names and argument types are fixed by this
// package, so an error here is a programming error.
func pack(a abi.ABI, method string, args ...any) []byte {
	data, err := a.Pack(method, args...)
	if err != nil {
		panic(fmt.Sprintf("pack %s: %v", method, err))
	}
	return data
}

// unpackOne decodes a single return value of type T. The ABI decoder already
// checks the types against the ABI; the checked assertion guards against an
// ABI file edited out of sync with this code.
func unpackOne[T any](a abi.ABI, method string, data []byte) (T, error) {
	var zero T
	vals, err := a.Unpack(method, data)
	if err != nil {
		return zero, err
	}
	if len(vals) != 1 {
		return zero, fmt.Errorf("%s: %d return values, want 1", method, len(vals))
	}
	v, ok := vals[0].(T)
	if !ok {
		return zero, fmt.Errorf("%s: unexpected return type %T", method, vals[0])
	}
	return v, nil
}

// unpackRound decodes (uint80, int256, uint256, uint256, uint80).
func unpackRound(method string, data []byte) (Round, error) {
	vals, err := aggregatorV3ABI.Unpack(method, data)
	if err != nil {
		return Round{}, err
	}
	if len(vals) != 5 {
		return Round{}, fmt.Errorf("%s: %d return values, want 5", method, len(vals))
	}
	ints := make([]*big.Int, 5)
	for i, v := range vals {
		b, ok := v.(*big.Int)
		if !ok {
			return Round{}, fmt.Errorf("%s: return %d has type %T", method, i, v)
		}
		ints[i] = b
	}
	return Round{RoundID: ints[0], Answer: ints[1], StartedAt: ints[2], UpdatedAt: ints[3], AnsweredInRound: ints[4]}, nil
}

func unpackAddress(a abi.ABI, method string, data []byte) (common.Address, error) {
	return unpackOne[common.Address](a, method, data)
}

// ABIs returns the embedded ABIs (AggregatorV3Interface, AggregatorProxy,
// AggregatorBounds), for test doubles that must answer them.
func ABIs() []abi.ABI { return []abi.ABI{aggregatorV3ABI, proxyABI, boundsABI} }
