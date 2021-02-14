package server

import (
	"context"
	"fmt"
	"log"
	"net"

	provider "github.com/lohmander/hostanapp/provider"
	"google.golang.org/grpc"
)

// EchoProviderServer implements the ProviderServer interface
type EchoProviderServer struct {
	provider.UnimplementedProviderServer
}

func (*EchoProviderServer) ProvisionAppConfig(ctx context.Context, req *provider.ProvisionAppConfigRequest) (*provider.ProvisionAppConfigResponse, error) {
	log.Printf("Returning config vars to %s", req.AppName)

	var configVars []*provider.ConfigVariable

	for k, v := range req.Config {
		configVars = append(configVars, &provider.ConfigVariable{
			Name:  k,
			Value: v,
		})
		configVars = append(configVars, &provider.ConfigVariable{
			Name:   fmt.Sprintf("secret-%s", k),
			Value:  v,
			Secret: true,
		})
	}

	return &provider.ProvisionAppConfigResponse{
		ConfigVariables: configVars,
	}, nil
}

func (*EchoProviderServer) DeprovisionAppConfig(ctx context.Context, req *provider.DeprovisionAppConfigRequest) (*provider.DeprovisionAppConfigResponse, error) {
	log.Printf("Deprovision called by %s", req.AppName)
	return &provider.DeprovisionAppConfigResponse{}, nil
}

func (*EchoProviderServer) Serve(port int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Listening on %d", port)

	var opts []grpc.ServerOption

	server := grpc.NewServer(opts...)
	provider.RegisterProviderServer(server, &EchoProviderServer{})
	server.Serve(lis)
}
