// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.37;

// Test doubles for the integration tests of oraclewatch. Not for production.
//
// The design follows the shape of Chainlink's MockV3Aggregator
// (@chainlink/contracts@1.5.0, src/v0.8/shared/mocks/MockV3Aggregator.sol),
// written from scratch and extended with what the monitor needs to exercise:
//   - every field of a round is settable, including zero and future values;
//   - decimals and description can change at runtime (metadata drift);
//   - minAnswer/maxAnswer and typeAndVersion as on OCR2/Dual aggregators;
//   - a proxy that encodes roundId as (phaseId << 64) | aggregatorRoundId and
//     can switch aggregator, incrementing the phase, like EACAggregatorProxy.

contract MockAggregator {
    struct Round {
        int256 answer;
        uint256 startedAt;
        uint256 updatedAt;
    }

    uint256 public constant version = 6;
    uint8 public decimals;
    string public description;
    string public typeAndVersion;
    int256 public minAnswer;
    int256 public maxAnswer;
    uint64 public latestRound;
    mapping(uint64 => Round) internal rounds;

    constructor(uint8 _decimals, string memory _description, int256 _minAnswer, int256 _maxAnswer, string memory _typeAndVersion) {
        decimals = _decimals;
        description = _description;
        minAnswer = _minAnswer;
        maxAnswer = _maxAnswer;
        typeAndVersion = _typeAndVersion;
    }

    /// Sets a round and makes it the latest one.
    function setRound(uint64 _roundId, int256 _answer, uint256 _startedAt, uint256 _updatedAt) external {
        rounds[_roundId] = Round(_answer, _startedAt, _updatedAt);
        latestRound = _roundId;
    }

    function setDecimals(uint8 _decimals) external {
        decimals = _decimals;
    }

    function setDescription(string calldata _description) external {
        description = _description;
    }

    function getRoundData(uint80 _roundId)
        external
        view
        returns (uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
    {
        // Like OCR2 aggregators, an unknown round returns zeros instead of reverting.
        Round memory r = rounds[uint64(_roundId)];
        return (_roundId, r.answer, r.startedAt, r.updatedAt, _roundId);
    }

    function latestRoundData()
        external
        view
        returns (uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
    {
        Round memory r = rounds[latestRound];
        return (latestRound, r.answer, r.startedAt, r.updatedAt, latestRound);
    }
}

contract MockProxy {
    uint256 private constant PHASE_OFFSET = 64;

    MockAggregator internal current;
    uint16 public phaseId;
    mapping(uint16 => address) public phaseAggregators;

    constructor(address _aggregator) {
        setAggregator(_aggregator);
    }

    /// Points the proxy at a new aggregator and starts a new phase.
    function setAggregator(address _aggregator) public {
        phaseId++;
        phaseAggregators[phaseId] = _aggregator;
        current = MockAggregator(_aggregator);
    }

    function aggregator() external view returns (address) {
        return address(current);
    }

    function decimals() external view returns (uint8) {
        return current.decimals();
    }

    function description() external view returns (string memory) {
        return current.description();
    }

    function version() external view returns (uint256) {
        return current.version();
    }

    function latestRoundData()
        external
        view
        returns (uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
    {
        (uint80 id, int256 a, uint256 s, uint256 u, ) = current.latestRoundData();
        uint80 proxyId = _encode(phaseId, uint64(id));
        return (proxyId, a, s, u, proxyId);
    }

    function getRoundData(uint80 _roundId)
        external
        view
        returns (uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)
    {
        uint16 phase = uint16(_roundId >> PHASE_OFFSET);
        uint64 aggRound = uint64(_roundId);
        (, int256 a, uint256 s, uint256 u, ) = MockAggregator(phaseAggregators[phase]).getRoundData(aggRound);
        return (_roundId, a, s, u, _roundId);
    }

    function _encode(uint16 _phase, uint64 _aggRound) internal pure returns (uint80) {
        return uint80((uint256(_phase) << PHASE_OFFSET) | _aggRound);
    }
}
