//go:build integration

// Code generated via abigen V2 - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package mockaggregator

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

// MockAggregatorMetaData contains all meta data concerning the MockAggregator contract.
var MockAggregatorMetaData = bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"_decimals\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"_description\",\"type\":\"string\"},{\"internalType\":\"int256\",\"name\":\"_minAnswer\",\"type\":\"int256\"},{\"internalType\":\"int256\",\"name\":\"_maxAnswer\",\"type\":\"int256\"},{\"internalType\":\"string\",\"name\":\"_typeAndVersion\",\"type\":\"string\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"description\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint80\",\"name\":\"_roundId\",\"type\":\"uint80\"}],\"name\":\"getRoundData\",\"outputs\":[{\"internalType\":\"uint80\",\"name\":\"roundId\",\"type\":\"uint80\"},{\"internalType\":\"int256\",\"name\":\"answer\",\"type\":\"int256\"},{\"internalType\":\"uint256\",\"name\":\"startedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"updatedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint80\",\"name\":\"answeredInRound\",\"type\":\"uint80\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"latestRound\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"latestRoundData\",\"outputs\":[{\"internalType\":\"uint80\",\"name\":\"roundId\",\"type\":\"uint80\"},{\"internalType\":\"int256\",\"name\":\"answer\",\"type\":\"int256\"},{\"internalType\":\"uint256\",\"name\":\"startedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"updatedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint80\",\"name\":\"answeredInRound\",\"type\":\"uint80\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"maxAnswer\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"minAnswer\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"_decimals\",\"type\":\"uint8\"}],\"name\":\"setDecimals\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_description\",\"type\":\"string\"}],\"name\":\"setDescription\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"_roundId\",\"type\":\"uint64\"},{\"internalType\":\"int256\",\"name\":\"_answer\",\"type\":\"int256\"},{\"internalType\":\"uint256\",\"name\":\"_startedAt\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"_updatedAt\",\"type\":\"uint256\"}],\"name\":\"setRound\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"typeAndVersion\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"version\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	ID:  "MockAggregator",
	Bin: "0x608060405234801561000f575f5ffd5b5060405161090038038061090083398101604081905261002e91610107565b5f805460ff191660ff871617905560016100488582610226565b5060038390556004829055600261005f8282610226565b5050505050506102e4565b634e487b7160e01b5f52604160045260245ffd5b5f82601f83011261008d575f5ffd5b81516001600160401b038111156100a6576100a661006a565b604051601f8201601f19908116603f011681016001600160401b03811182821017156100d4576100d461006a565b6040528181528382016020018510156100eb575f5ffd5b8160208501602083015e5f918101602001919091529392505050565b5f5f5f5f5f60a0868803121561011b575f5ffd5b855160ff8116811461012b575f5ffd5b60208701519095506001600160401b03811115610146575f5ffd5b6101528882890161007e565b6040880151606089015160808a0151929750909550935090506001600160401b0381111561017e575f5ffd5b61018a8882890161007e565b9150509295509295909350565b600181811c908216806101ab57607f821691505b6020821081036101c957634e487b7160e01b5f52602260045260245ffd5b50919050565b601f821115610221578282111561022157805f5260205f20601f840160051c60208510156101fa57505f5b90810190601f840160051c035f5b8181101561021d575f83820155600101610208565b5050505b505050565b81516001600160401b0381111561023f5761023f61006a565b6102538161024d8454610197565b846101cf565b6020601f821160018114610285575f831561026e5750848201515b5f19600385901b1c1916600184901b1784556102dd565b5f84815260208120601f198516915b828110156102b45787850151825560209485019460019092019101610294565b50848210156102d157868401515f19600387901b60f8161c191681555b505060018360011b0184555b5050505050565b61060f806102f15f395ff3fe608060405234801561000f575f5ffd5b50600436106100b1575f3560e01c80637284e4161161006e5780637284e416146101465780637a1395aa1461014e5780638e9297091461017357806390c3f38f146101db5780639a6fc8f5146101ee578063feaf968c1461027b575f5ffd5b8063181f5a77146100b557806322adbc78146100d3578063313ce567146100ea57806354fd4d5014610108578063668a0f021461011057806370da2f671461013d575b5f5ffd5b6100bd6102c3565b6040516100ca919061036e565b60405180910390f35b6100dc60035481565b6040519081526020016100ca565b5f546100f69060ff1681565b60405160ff90911681526020016100ca565b6100dc600681565b6005546101249067ffffffffffffffff1681565b60405167ffffffffffffffff90911681526020016100ca565b6100dc60045481565b6100bd61034f565b61017161015c3660046103a3565b5f805460ff191660ff92909216919091179055565b005b6101716101813660046103ca565b60408051606081018252938452602080850193845284820192835267ffffffffffffffff9095165f818152600690965294209251835590516001830155516002909101556005805467ffffffffffffffff19169091179055565b6101716101e936600461040e565b61035c565b6102446101fc36600461047c565b67ffffffffffffffff81165f90815260066020908152604091829020825160608101845281548082526001830154938201849052600290920154930183905292939091908490565b6040805169ffffffffffffffffffff968716815260208101959095528401929092526060830152909116608082015260a0016100ca565b60055467ffffffffffffffff165f8181526006602090815260409182902082516060810184528154808252600183015493820184905260029092015493018390529183610244565b600280546102d0906104a5565b80601f01602080910402602001604051908101604052809291908181526020018280546102fc906104a5565b80156103475780601f1061031e57610100808354040283529160200191610347565b820191905f5260205f20905b81548152906001019060200180831161032a57829003601f168201915b505050505081565b600180546102d0906104a5565b6001610369828483610547565b505050565b602081525f82518060208401528060208501604085015e5f604082850101526040601f19601f83011684010191505092915050565b5f602082840312156103b3575f5ffd5b813560ff811681146103c3575f5ffd5b9392505050565b5f5f5f5f608085870312156103dd575f5ffd5b843567ffffffffffffffff811681146103f4575f5ffd5b966020860135965060408601359560600135945092505050565b5f5f6020838503121561041f575f5ffd5b823567ffffffffffffffff811115610435575f5ffd5b8301601f81018513610445575f5ffd5b803567ffffffffffffffff81111561045b575f5ffd5b85602082840101111561046c575f5ffd5b6020919091019590945092505050565b5f6020828403121561048c575f5ffd5b813569ffffffffffffffffffff811681146103c3575f5ffd5b600181811c908216806104b957607f821691505b6020821081036104d757634e487b7160e01b5f52602260045260245ffd5b50919050565b634e487b7160e01b5f52604160045260245ffd5b601f821115610369578282111561036957805f5260205f20601f840160051c602085101561051c57505f5b90810190601f840160051c035f5b8181101561053f575f8382015560010161052a565b505050505050565b67ffffffffffffffff83111561055f5761055f6104dd565b6105738361056d83546104a5565b836104f1565b5f601f8411600181146105a4575f851561058d5750838201355b5f19600387901b1c1916600186901b1783556105fb565b5f83815260208120601f198716915b828110156105d357868501358255602094850194600190920191016105b3565b50868210156105ef575f1960f88860031b161c19848701351681555b505060018560011b0183555b505050505056fea164736f6c6343000825000a",
}

// MockAggregator is an auto generated Go binding around an Ethereum contract.
type MockAggregator struct {
	abi abi.ABI
}

// GetABI returns the ABI associated with this contract binding.
func (c *MockAggregator) GetABI() abi.ABI {
	return c.abi
}

// NewMockAggregator creates a new instance of MockAggregator.
func NewMockAggregator() *MockAggregator {
	parsed, err := MockAggregatorMetaData.ParseABI()
	if err != nil {
		panic(errors.New("invalid ABI: " + err.Error()))
	}
	return &MockAggregator{abi: *parsed}
}

// Instance creates a wrapper for a deployed contract instance at the given address.
// Use this to create the instance object passed to abigen v2 library functions Call, Transact, etc.
func (c *MockAggregator) Instance(backend bind.ContractBackend, addr common.Address) *bind.BoundContract {
	return bind.NewBoundContract(addr, c.abi, backend, backend, backend)
}

// PackConstructor is the Go binding used to pack the parameters required for
// contract deployment.
//
// Solidity: constructor(uint8 _decimals, string _description, int256 _minAnswer, int256 _maxAnswer, string _typeAndVersion) returns()
func (mockAggregator *MockAggregator) PackConstructor(_decimals uint8, _description string, _minAnswer *big.Int, _maxAnswer *big.Int, _typeAndVersion string) []byte {
	enc, err := mockAggregator.abi.Pack("", _decimals, _description, _minAnswer, _maxAnswer, _typeAndVersion)
	if err != nil {
		panic(err)
	}
	return enc
}

// PackDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x313ce567.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function decimals() view returns(uint8)
func (mockAggregator *MockAggregator) PackDecimals() []byte {
	enc, err := mockAggregator.abi.Pack("decimals")
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
func (mockAggregator *MockAggregator) TryPackDecimals() ([]byte, error) {
	return mockAggregator.abi.Pack("decimals")
}

// UnpackDecimals is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (mockAggregator *MockAggregator) UnpackDecimals(data []byte) (uint8, error) {
	out, err := mockAggregator.abi.Unpack("decimals", data)
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
func (mockAggregator *MockAggregator) PackDescription() []byte {
	enc, err := mockAggregator.abi.Pack("description")
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
func (mockAggregator *MockAggregator) TryPackDescription() ([]byte, error) {
	return mockAggregator.abi.Pack("description")
}

// UnpackDescription is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x7284e416.
//
// Solidity: function description() view returns(string)
func (mockAggregator *MockAggregator) UnpackDescription(data []byte) (string, error) {
	out, err := mockAggregator.abi.Unpack("description", data)
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
func (mockAggregator *MockAggregator) PackGetRoundData(roundId *big.Int) []byte {
	enc, err := mockAggregator.abi.Pack("getRoundData", roundId)
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
func (mockAggregator *MockAggregator) TryPackGetRoundData(roundId *big.Int) ([]byte, error) {
	return mockAggregator.abi.Pack("getRoundData", roundId)
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
func (mockAggregator *MockAggregator) UnpackGetRoundData(data []byte) (GetRoundDataOutput, error) {
	out, err := mockAggregator.abi.Unpack("getRoundData", data)
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

// PackLatestRound is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x668a0f02.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function latestRound() view returns(uint64)
func (mockAggregator *MockAggregator) PackLatestRound() []byte {
	enc, err := mockAggregator.abi.Pack("latestRound")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackLatestRound is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x668a0f02.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function latestRound() view returns(uint64)
func (mockAggregator *MockAggregator) TryPackLatestRound() ([]byte, error) {
	return mockAggregator.abi.Pack("latestRound")
}

// UnpackLatestRound is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x668a0f02.
//
// Solidity: function latestRound() view returns(uint64)
func (mockAggregator *MockAggregator) UnpackLatestRound(data []byte) (uint64, error) {
	out, err := mockAggregator.abi.Unpack("latestRound", data)
	if err != nil {
		return *new(uint64), err
	}
	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)
	return out0, nil
}

// PackLatestRoundData is the Go binding used to pack the parameters required for calling
// the contract method with ID 0xfeaf968c.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function latestRoundData() view returns(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
func (mockAggregator *MockAggregator) PackLatestRoundData() []byte {
	enc, err := mockAggregator.abi.Pack("latestRoundData")
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
func (mockAggregator *MockAggregator) TryPackLatestRoundData() ([]byte, error) {
	return mockAggregator.abi.Pack("latestRoundData")
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
func (mockAggregator *MockAggregator) UnpackLatestRoundData(data []byte) (LatestRoundDataOutput, error) {
	out, err := mockAggregator.abi.Unpack("latestRoundData", data)
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

// PackMaxAnswer is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x70da2f67.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function maxAnswer() view returns(int256)
func (mockAggregator *MockAggregator) PackMaxAnswer() []byte {
	enc, err := mockAggregator.abi.Pack("maxAnswer")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMaxAnswer is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x70da2f67.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function maxAnswer() view returns(int256)
func (mockAggregator *MockAggregator) TryPackMaxAnswer() ([]byte, error) {
	return mockAggregator.abi.Pack("maxAnswer")
}

// UnpackMaxAnswer is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x70da2f67.
//
// Solidity: function maxAnswer() view returns(int256)
func (mockAggregator *MockAggregator) UnpackMaxAnswer(data []byte) (*big.Int, error) {
	out, err := mockAggregator.abi.Unpack("maxAnswer", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackMinAnswer is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22adbc78.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function minAnswer() view returns(int256)
func (mockAggregator *MockAggregator) PackMinAnswer() []byte {
	enc, err := mockAggregator.abi.Pack("minAnswer")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackMinAnswer is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x22adbc78.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function minAnswer() view returns(int256)
func (mockAggregator *MockAggregator) TryPackMinAnswer() ([]byte, error) {
	return mockAggregator.abi.Pack("minAnswer")
}

// UnpackMinAnswer is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x22adbc78.
//
// Solidity: function minAnswer() view returns(int256)
func (mockAggregator *MockAggregator) UnpackMinAnswer(data []byte) (*big.Int, error) {
	out, err := mockAggregator.abi.Unpack("minAnswer", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}

// PackSetDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7a1395aa.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDecimals(uint8 _decimals) returns()
func (mockAggregator *MockAggregator) PackSetDecimals(decimals uint8) []byte {
	enc, err := mockAggregator.abi.Pack("setDecimals", decimals)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDecimals is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x7a1395aa.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDecimals(uint8 _decimals) returns()
func (mockAggregator *MockAggregator) TryPackSetDecimals(decimals uint8) ([]byte, error) {
	return mockAggregator.abi.Pack("setDecimals", decimals)
}

// PackSetDescription is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x90c3f38f.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setDescription(string _description) returns()
func (mockAggregator *MockAggregator) PackSetDescription(description string) []byte {
	enc, err := mockAggregator.abi.Pack("setDescription", description)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetDescription is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x90c3f38f.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setDescription(string _description) returns()
func (mockAggregator *MockAggregator) TryPackSetDescription(description string) ([]byte, error) {
	return mockAggregator.abi.Pack("setDescription", description)
}

// PackSetRound is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8e929709.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function setRound(uint64 _roundId, int256 _answer, uint256 _startedAt, uint256 _updatedAt) returns()
func (mockAggregator *MockAggregator) PackSetRound(roundId uint64, answer *big.Int, startedAt *big.Int, updatedAt *big.Int) []byte {
	enc, err := mockAggregator.abi.Pack("setRound", roundId, answer, startedAt, updatedAt)
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackSetRound is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x8e929709.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function setRound(uint64 _roundId, int256 _answer, uint256 _startedAt, uint256 _updatedAt) returns()
func (mockAggregator *MockAggregator) TryPackSetRound(roundId uint64, answer *big.Int, startedAt *big.Int, updatedAt *big.Int) ([]byte, error) {
	return mockAggregator.abi.Pack("setRound", roundId, answer, startedAt, updatedAt)
}

// PackTypeAndVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x181f5a77.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function typeAndVersion() view returns(string)
func (mockAggregator *MockAggregator) PackTypeAndVersion() []byte {
	enc, err := mockAggregator.abi.Pack("typeAndVersion")
	if err != nil {
		panic(err)
	}
	return enc
}

// TryPackTypeAndVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x181f5a77.  This method will return an error
// if any inputs are invalid/nil.
//
// Solidity: function typeAndVersion() view returns(string)
func (mockAggregator *MockAggregator) TryPackTypeAndVersion() ([]byte, error) {
	return mockAggregator.abi.Pack("typeAndVersion")
}

// UnpackTypeAndVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x181f5a77.
//
// Solidity: function typeAndVersion() view returns(string)
func (mockAggregator *MockAggregator) UnpackTypeAndVersion(data []byte) (string, error) {
	out, err := mockAggregator.abi.Unpack("typeAndVersion", data)
	if err != nil {
		return *new(string), err
	}
	out0 := *abi.ConvertType(out[0], new(string)).(*string)
	return out0, nil
}

// PackVersion is the Go binding used to pack the parameters required for calling
// the contract method with ID 0x54fd4d50.  This method will panic if any
// invalid/nil inputs are passed.
//
// Solidity: function version() view returns(uint256)
func (mockAggregator *MockAggregator) PackVersion() []byte {
	enc, err := mockAggregator.abi.Pack("version")
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
func (mockAggregator *MockAggregator) TryPackVersion() ([]byte, error) {
	return mockAggregator.abi.Pack("version")
}

// UnpackVersion is the Go binding that unpacks the parameters returned
// from invoking the contract method with ID 0x54fd4d50.
//
// Solidity: function version() view returns(uint256)
func (mockAggregator *MockAggregator) UnpackVersion(data []byte) (*big.Int, error) {
	out, err := mockAggregator.abi.Unpack("version", data)
	if err != nil {
		return new(big.Int), err
	}
	out0 := abi.ConvertType(out[0], new(big.Int)).(*big.Int)
	return out0, nil
}
