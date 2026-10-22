//go:build integration

// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package mockproxy

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = bytes.Equal
	_ = errors.New
	_ = big.NewInt
	_ = common.Big1
	_ = types.BloomLookup
	_ = abi.ConvertType
)

// MockProxyMetaData contains all meta data concerning the MockProxy contract.
var MockProxyMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_aggregator\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"aggregator\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"description\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint80\",\"name\":\"_roundId\",\"type\":\"uint80\"}],\"name\":\"getRoundData\",\"outputs\":[{\"internalType\":\"uint80\",\"name\":\"roundId\",\"type\":\"uint80\"},{\"internalType\":\"int256\",\"name\":\"answer\",\"type\":\"int256\"},{\"internalType\":\"uint256\",\"name\":\"startedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"updatedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint80\",\"name\":\"answeredInRound\",\"type\":\"uint80\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"latestRoundData\",\"outputs\":[{\"internalType\":\"uint80\",\"name\":\"roundId\",\"type\":\"uint80\"},{\"internalType\":\"int256\",\"name\":\"answer\",\"type\":\"int256\"},{\"internalType\":\"uint256\",\"name\":\"startedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"updatedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint80\",\"name\":\"answeredInRound\",\"type\":\"uint80\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint16\",\"name\":\"\",\"type\":\"uint16\"}],\"name\":\"phaseAggregators\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"phaseId\",\"outputs\":[{\"internalType\":\"uint16\",\"name\":\"\",\"type\":\"uint16\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_aggregator\",\"type\":\"address\"}],\"name\":\"setAggregator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"version\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "MockProxy",
	Bin: "0x608060405234801561000f575f5ffd5b5060405161085538038061085583398101604081905261002e916100b6565b6100378161003d565b5061010f565b5f8054600160a01b900461ffff16906014610057836100e3565b82546101009290920a61ffff8181021990931691831602179091555f8054600160a01b9004909116815260016020526040812080546001600160a01b03199081166001600160a01b039590951694851790915581541690921790915550565b5f602082840312156100c6575f5ffd5b81516001600160a01b03811681146100dc575f5ffd5b9392505050565b5f61ffff821661ffff810361010657634e487b7160e01b5f52601160045260245ffd5b60010192915050565b6107398061011c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610090575f3560e01c80637284e416116100635780637284e416146101145780639a6fc8f514610129578063c159730414610173578063f9120af61461019b578063feaf968c146101b0575f5ffd5b8063245a7bfc14610094578063313ce567146100bd57806354fd4d50146100d757806358303b10146100ed575b5f5ffd5b5f546001600160a01b03165b6040516001600160a01b0390911681526020015b60405180910390f35b6100c56101b8565b60405160ff90911681526020016100b4565b6100df610231565b6040519081526020016100b4565b5f5461010190600160a01b900461ffff1681565b60405161ffff90911681526020016100b4565b61011c6102a5565b6040516100b491906104f6565b61013c610137366004610545565b610312565b6040805169ffffffffffffffffffff968716815260208101959095528401929092526060830152909116608082015260a0016100b4565b6100a0610181366004610567565b60016020525f90815260409020546001600160a01b031681565b6101ae6101a9366004610588565b6103c3565b005b61013c61043c565b5f5f5f9054906101000a90046001600160a01b03166001600160a01b031663313ce5676040518163ffffffff1660e01b8152600401602060405180830381865afa158015610208573d5f5f3e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061022c91906105ae565b905090565b5f5f5f9054906101000a90046001600160a01b03166001600160a01b03166354fd4d506040518163ffffffff1660e01b8152600401602060405180830381865afa158015610281573d5f5f3e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061022c91906105ce565b5f805460408051633942720b60e11b815290516060936001600160a01b0390931692637284e41692600480820193918290030181865afa1580156102eb573d5f5f3e3d5ffd5b505050506040513d5f823e601f3d908101601f1916820160405261022c91908101906105f9565b604081811c61ffff165f8181526001602052828120549251639a6fc8f560e01b815267ffffffffffffffff85166004820152909283928392839283929188918491829182916001600160a01b0390911690639a6fc8f59060240160a060405180830381865afa158015610387573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906103ab91906106ac565b509d9e919d909c509a508d9950975050505050505050565b5f8054600160a01b900461ffff169060146103dd83610700565b82546101009290920a61ffff8181021990931691831602179091555f8054600160a01b9004909116815260016020526040812080546001600160a01b03199081166001600160a01b039590951694851790915581541690921790915550565b5f5f5f5f5f5f5f5f5f5f5f9054906101000a90046001600160a01b03166001600160a01b031663feaf968c6040518163ffffffff1660e01b815260040160a060405180830381865afa158015610494573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906104b891906106ac565b505f5467ffffffffffffffff909416600160a01b90940460401b69ffff000000000000000016939093179c919b5099509097508a9650945050505050565b602081525f82518060208401528060208501604085015e5f604082850101526040601f19601f83011684010191505092915050565b69ffffffffffffffffffff81168114610542575f5ffd5b50565b5f60208284031215610555575f5ffd5b81356105608161052b565b9392505050565b5f60208284031215610577575f5ffd5b813561ffff81168114610560575f5ffd5b5f60208284031215610598575f5ffd5b81356001600160a01b0381168114610560575f5ffd5b5f602082840312156105be575f5ffd5b815160ff81168114610560575f5ffd5b5f602082840312156105de575f5ffd5b5051919050565b634e487b7160e01b5f52604160045260245ffd5b5f60208284031215610609575f5ffd5b815167ffffffffffffffff81111561061f575f5ffd5b8201601f8101841361062f575f5ffd5b805167ffffffffffffffff811115610649576106496105e5565b604051601f8201601f19908116603f0116810167ffffffffffffffff81118282101715610678576106786105e5565b60405281815282820160200186101561068f575f5ffd5b8160208401602083015e5f91810160200191909152949350505050565b5f5f5f5f5f60a086880312156106c0575f5ffd5b85516106cb8161052b565b60208701516040880151606089015160808a0151939850919650945092506106f28161052b565b809150509295509295909350565b5f61ffff821661ffff810361072357634e487b7160e01b5f52601160045260245ffd5b6001019291505056fea164736f6c6343000825000a",
}

// MockProxy is an auto generated Go binding around an Ethereum contract.
type MockProxy struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *MockProxy) GetABI() abi.ABI {
	return c.abi
}

// NewMockProxy creates a new instance of MockProxy.
func NewMockProxy() *MockProxy {
	parsed, err := MockProxyMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &MockProxy{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *MockProxy) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(address _aggregator) returns()
func (mockProxy *MockProxy) PackConstructor(_aggregator common.Address) []byte {
	enc, err := mockProxy.abi.Pack("", _aggregator)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackAggregator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x245a7bfc.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function aggregator() view returns(address)
func (mockProxy *MockProxy) PackAggregator() []byte {
	enc, err := mockProxy.abi.Pack("aggregator")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackAggregator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x245a7bfc.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function aggregator() view returns(address)
func (mockProxy *MockProxy) TryPackAggregator() ([]byte, error) {
	return mockProxy.abi.Pack("aggregator")
}

// UnpackAggregator is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x245a7bfc.
//
// Solidity: function aggregator() view returns(address)
func (mockProxy *MockProxy) UnpackAggregator(data []byte) (common.Address, error) {
	out, err := mockProxy.abi.Unpack("aggregator", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function decimals() view returns(uint8)
func (mockProxy *MockProxy) PackDecimals() []byte {
	enc, err := mockProxy.abi.Pack("decimals")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function decimals() view returns(uint8)
func (mockProxy *MockProxy) TryPackDecimals() ([]byte, error) {
	return mockProxy.abi.Pack("decimals")
}

// UnpackDecimals is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (mockProxy *MockProxy) UnpackDecimals(data []byte) (uint8, error) {
	out, err := mockProxy.abi.Unpack("decimals", data)
	if err != nil {
		return *new(uint8), err
	}
	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)
	return out0, nil
}

// PackDescription is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7284e416.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function description() view returns(string)
func (mockProxy *MockProxy) PackDescription() []byte {
	enc, err := mockProxy.abi.Pack("description")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackDescription is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7284e416.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function description() view returns(string)
func (mockProxy *MockProxy) TryPackDescription() ([]byte, error) {
	return mockProxy.abi.Pack("description")
}

// UnpackDescription is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x7284e416.
//
// Solidity: function description() view returns(string)
func (mockProxy *MockProxy) UnpackDescription(data []byte) (string, error) {
	out, err := mockProxy.abi.Unpack("description", data)
	if err != nil {
		return *new(string), err
	}
	out0 := *abi.ConvertType(out[0], new(string)).(*string)
	return out0, nil
}

// PackGetRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9a6fc8f5.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function getRoundData(uint80 _roundId) view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) PackGetRoundData(roundId *big.Int) []byte {
	enc, err := mockProxy.abi.Pack("getRoundData", roundId)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackGetRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x9a6fc8f5.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function getRoundData(uint80 _roundId) view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) TryPackGetRoundData(roundId *big.Int) ([]byte, error) {
	return mockProxy.abi.Pack("getRoundData", roundId)
}

// GetRoundDataOutput serves as a container for the return parameters of contract
// method GetRoundData.
type GetRoundDataOutput struct {
	RoundId         *big.Int
	Answer          *big.Int
	StartedAt       *big.Int
	UpdatedAt       *big.Int
	AnsweredInRound *big.Int
}

// UnpackGetRoundData is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x9a6fc8f5.
//
// Solidity: function getRoundData(uint80 _roundId) view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) UnpackGetRoundData(data []byte) (GetRoundDataOutput, error) {
	out, err := mockProxy.abi.Unpack("getRoundData", data)
	outstruct := new(GetRoundDataOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.RoundId = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Answer = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.StartedAt = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.UpdatedAt = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.AnsweredInRound = abi.ConvertType(out[4], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackLatestRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfeaf968c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) PackLatestRoundData() []byte {
	enc, err := mockProxy.abi.Pack("latestRoundData")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLatestRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfeaf968c.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) TryPackLatestRoundData() ([]byte, error) {
	return mockProxy.abi.Pack("latestRoundData")
}

// LatestRoundDataOutput serves as a container for the return parameters of contract
// method LatestRoundData.
type LatestRoundDataOutput struct {
	RoundId         *big.Int
	Answer          *big.Int
	StartedAt       *big.Int
	UpdatedAt       *big.Int
	AnsweredInRound *big.Int
}

// UnpackLatestRoundData is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xfeaf968c.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockProxy *MockProxy) UnpackLatestRoundData(data []byte) (LatestRoundDataOutput, error) {
	out, err := mockProxy.abi.Unpack("latestRoundData", data)
	outstruct := new(LatestRoundDataOutput)
	if err != nil {
		return *outstruct, err
	}
	outstruct.RoundId = abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	outstruct.Answer = abi.ConvertType(out[1], new(big.Int)).(*big.Int)
	outstruct.StartedAt = abi.ConvertType(out[2], new(big.Int)).(*big.Int)
	outstruct.UpdatedAt = abi.ConvertType(out[3], new(big.Int)).(*big.Int)
	outstruct.AnsweredInRound = abi.ConvertType(out[4], new(big.Int)).(*big.Int)
	return *outstruct, nil
}

// PackPhaseAggregators is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc1597304.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function phaseAggregators(uint16 ) view returns(address)
func (mockProxy *MockProxy) PackPhaseAggregators(arg0 uint16) []byte {
	enc, err := mockProxy.abi.Pack("phaseAggregators", arg0)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPhaseAggregators is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xc1597304.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function phaseAggregators(uint16 ) view returns(address)
func (mockProxy *MockProxy) TryPackPhaseAggregators(arg0 uint16) ([]byte, error) {
	return mockProxy.abi.Pack("phaseAggregators", arg0)
}

// UnpackPhaseAggregators is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0xc1597304.
//
// Solidity: function phaseAggregators(uint16 ) view returns(address)
func (mockProxy *MockProxy) UnpackPhaseAggregators(data []byte) (common.Address, error) {
	out, err := mockProxy.abi.Unpack("phaseAggregators", data)
	if err != nil {
		return *new(common.Address), err
	}
	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

// PackPhaseId is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x58303b10.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function phaseId() view returns(uint16)
func (mockProxy *MockProxy) PackPhaseId() []byte {
	enc, err := mockProxy.abi.Pack("phaseId")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackPhaseId is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x58303b10.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function phaseId() view returns(uint16)
func (mockProxy *MockProxy) TryPackPhaseId() ([]byte, error) {
	return mockProxy.abi.Pack("phaseId")
}

// UnpackPhaseId is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x58303b10.
//
// Solidity: function phaseId() view returns(uint16)
func (mockProxy *MockProxy) UnpackPhaseId(data []byte) (uint16, error) {
	out, err := mockProxy.abi.Unpack("phaseId", data)
	if err != nil {
		return *new(uint16), err
	}
	out0 := *abi.ConvertType(out[0], new(uint16)).(*uint16)
	return out0, nil
}

// PackSetAggregator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf9120af6.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setAggregator(address _aggregator) returns()
func (mockProxy *MockProxy) PackSetAggregator(aggregator common.Address) []byte {
	enc, err := mockProxy.abi.Pack("setAggregator", aggregator)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetAggregator is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xf9120af6.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setAggregator(address _aggregator) returns()
func (mockProxy *MockProxy) TryPackSetAggregator(aggregator common.Address) ([]byte, error) {
	return mockProxy.abi.Pack("setAggregator", aggregator)
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint256)
func (mockProxy *MockProxy) PackVersion() []byte {
	enc, err := mockProxy.abi.Pack("version")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function version() view returns(uint256)
func (mockProxy *MockProxy) TryPackVersion() ([]byte, error) {
	return mockProxy.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint256)
func (mockProxy *MockProxy) UnpackVersion(data []byte) (*big.Int, error) {
	out, err := mockProxy.abi.Unpack("version", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
