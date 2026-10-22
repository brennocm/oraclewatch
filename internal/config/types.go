package config

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration written in YAML as a Go duration string ("90s", "1h").
type Duration struct{ time.Duration }

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("line %d: duration must be a string like \"60s\"", n.Line)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q", n.Line, s)
	}
	d.Duration = v
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

// Percent is a ratio written in YAML as a percentage ("0.5%"). It is stored as
// an exact rational (0.5% is 1/200), so values such as 0.1% that have no exact
// binary floating-point representation compare exactly.
type Percent struct{ rat *big.Rat }

// ParsePercent parses "0.5%". The percent sign is required so that "0.5" is
// never silently read as 50%.
func ParsePercent(s string) (Percent, error) {
	s = strings.TrimSpace(s)
	num, ok := strings.CutSuffix(s, "%")
	if !ok {
		return Percent{}, fmt.Errorf("percentage %q must end with %%", s)
	}
	num = strings.TrimSpace(num)
	// big.Rat.SetString also accepts fractions ("1/3") and exponents; only
	// plain decimals are accepted here to keep configs unambiguous.
	if num == "" || strings.ContainsAny(num, "/eE") {
		return Percent{}, fmt.Errorf("invalid percentage %q", s)
	}
	r, ok := new(big.Rat).SetString(num)
	if !ok {
		return Percent{}, fmt.Errorf("invalid percentage %q", s)
	}
	return Percent{rat: r.Quo(r, big.NewRat(100, 1))}, nil
}

// MustPercent is ParsePercent for constants; it panics on invalid input.
func MustPercent(s string) Percent {
	p, err := ParsePercent(s)
	if err != nil {
		panic(err)
	}
	return p
}

// Rat returns the ratio as a new big.Rat (0.5% → 1/200). The zero Percent is 0.
func (p Percent) Rat() *big.Rat {
	if p.rat == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(p.rat)
}

// IsSet reports whether the value was set (from YAML or a default).
func (p Percent) IsSet() bool { return p.rat != nil }

// String formats the value as a percentage with up to 4 decimals.
func (p Percent) String() string {
	r := p.Rat()
	r.Mul(r, big.NewRat(100, 1))
	s := r.FloatString(4)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + "%"
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *Percent) UnmarshalYAML(n *yaml.Node) error {
	v, err := ParsePercent(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*p = v
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (p Percent) MarshalYAML() (any, error) { return p.String(), nil }

// Secret is a configuration value that may contain credentials (RPC URLs,
// webhook URLs, tokens). It is the only type whose ${VAR} references are
// expanded from the environment, and it never prints its value.
type Secret struct {
	raw   string // as written in the file, e.g. "${ETH_RPC_URL}"
	value string // after expansion
}

// NewSecret returns an already-expanded secret; intended for tests and code
// that builds a Config programmatically.
func NewSecret(v string) Secret { return Secret{raw: v, value: v} }

// Value returns the expanded secret. Callers must not log it.
func (s Secret) Value() string { return s.value }

// IsZero reports whether the secret is empty after expansion.
func (s Secret) IsZero() bool { return s.value == "" }

// String never reveals the value.
func (s Secret) String() string {
	if s.value == "" {
		return ""
	}
	return "[REDACTED]"
}

// GoString keeps %#v from printing the value.
func (s Secret) GoString() string { return "config.Secret(" + s.String() + ")" }

// UnmarshalYAML implements yaml.Unmarshaler. Expansion happens later, in Load,
// with an explicit environment lookup function.
func (s *Secret) UnmarshalYAML(n *yaml.Node) error {
	var v string
	if err := n.Decode(&v); err != nil {
		return fmt.Errorf("line %d: secret must be a string", n.Line)
	}
	s.raw = v
	return nil
}

// MarshalYAML implements yaml.Marshaler; it writes the unexpanded form.
func (s Secret) MarshalYAML() (any, error) { return s.raw, nil }

// errUndefinedVar is returned when a ${VAR} reference has no value.
var errUndefinedVar = errors.New("environment variable is not set")

// expand resolves ${NAME} references in raw. Any other use of "$" is an error,
// so that a literal "$" in a token is never half-expanded by accident.
func expand(raw string, lookup func(string) (string, bool)) (string, error) {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '$' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(raw) || raw[i+1] != '{' {
			return "", errors.New(`"$" must start a ${NAME} reference`)
		}
		end := strings.IndexByte(raw[i+2:], '}')
		if end < 0 {
			return "", errors.New("unterminated ${ reference")
		}
		name := raw[i+2 : i+2+end]
		if !validEnvName(name) {
			return "", fmt.Errorf("invalid variable name %q", name)
		}
		v, ok := lookup(name)
		if !ok {
			return "", fmt.Errorf("%w: %s", errUndefinedVar, name)
		}
		b.WriteString(v)
		i += 2 + end
	}
	return b.String(), nil
}

func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c == '_', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// Address is an EVM address that must be written with a valid EIP-55 checksum.
type Address struct {
	common.Address
	set bool
}

// ParseAddress accepts only "0x" + 40 hex digits in EIP-55 checksum form.
// All-lowercase addresses are rejected too: the checksum is the only protection
// against a mistyped address, and every address in the official data files is
// checksummed.
func ParseAddress(s string) (Address, error) {
	if !common.IsHexAddress(s) || !strings.HasPrefix(s, "0x") {
		return Address{}, fmt.Errorf("%q is not a 0x-prefixed 20-byte hex address", s)
	}
	a := common.HexToAddress(s)
	if a.Hex() != s {
		return Address{}, fmt.Errorf("%q fails the EIP-55 checksum (expected %s)", s, a.Hex())
	}
	if a == (common.Address{}) {
		return Address{}, errors.New("zero address is not allowed")
	}
	return Address{Address: a, set: true}, nil
}

// IsSet reports whether an address was configured.
func (a Address) IsSet() bool { return a.set }

// UnmarshalYAML implements yaml.Unmarshaler.
func (a *Address) UnmarshalYAML(n *yaml.Node) error {
	v, err := ParseAddress(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*a = v
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (a Address) MarshalYAML() (any, error) { return a.Hex(), nil }
