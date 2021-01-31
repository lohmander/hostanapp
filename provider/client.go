package provider

import (
	"google.golang.org/grpc"
)

// NewClient returns a new ProviderClient instance with the
// provided url
func NewClient(url string) (ProviderClient, error) {
	var opts []grpc.DialOption

	opts = append(opts, grpc.WithInsecure())

	conn, err := grpc.Dial(url, opts...)

	if err != nil {
		return nil, err
	}

	return NewProviderClient(conn), nil
}
