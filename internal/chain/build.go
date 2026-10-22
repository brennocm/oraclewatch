package chain

import (
	"context"
	"fmt"

	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// BuildOptions are shared by every endpoint built from the configuration.
type BuildOptions struct {
	Clock    clock.Clock
	Observer Observer
	Redactor *redact.Redactor
}

// NewNetworkFromConfig creates the endpoints of a configured network. It makes
// no network calls; call VerifyChainIDs before the first cycle.
func NewNetworkFromConfig(ctx context.Context, n *config.Network, o BuildOptions) (*Network, error) {
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	net := &Network{
		Name:          n.Name,
		ChainID:       n.ChainID,
		Confirmations: n.Confirmations,
		MaxHeadLag:    n.MaxHeadLag.Duration,
		Clock:         o.Clock,
		// Never read from an endpoint before its chain ID was confirmed.
		RequireVerified: true,
	}
	for _, ep := range n.RPCEndpoints {
		b, err := DialHTTP(ctx, ep.URL.Value())
		if err != nil {
			// The dial error may echo the URL.
			return nil, fmt.Errorf("network %s endpoint %s: %w", n.Name, ep.Name, o.Redactor.Error(err))
		}
		net.Endpoints = append(net.Endpoints, NewEndpoint(n.Name, ep.Name, b, EndpointOptions{
			Timeout:  n.CallTimeout.Duration,
			Clock:    o.Clock,
			Observer: o.Observer,
			Redactor: o.Redactor,
		}))
	}
	return net, nil
}
