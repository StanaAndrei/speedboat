package transport

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const DefaultConnectionTimeout = 10 * time.Second

func Connect(natsURL string, opts ...nats.Option) (*nats.Conn, nats.JetStreamContext, error) {
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	defaultOpts := []nats.Option{
		nats.Timeout(DefaultConnectionTimeout),
		nats.ReconnectWait(1 * time.Second),
		nats.MaxReconnects(-1),
	}
	defaultOpts = append(defaultOpts, opts...)

	nc, err := nats.Connect(natsURL, defaultOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}
	return nc, js, nil
}
