package main

import (
	"context"
	"fmt"
	"log"
	"net"

	provider "github.com/lohmander/hostanapp/provider"
	"google.golang.org/grpc"
)

type HelloProviderServer struct {
	provider.UnimplementedProviderServer
}

func (*HelloProviderServer) ProvisionAppConfig(ctx context.Context, req *provider.ProvisionAppConfigRequest) (*provider.ProvisionAppConfigResponse, error) {
	log.Printf("Returning config vars to %s", req.AppName)

	return &provider.ProvisionAppConfigResponse{
		ConfigVariables: []*provider.ConfigVariable{
			{Name: "hello", Value: req.AppName, Secret: false},
		},
	}, nil
}

func main() {
	port := 5000
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Listening on %d", port)

	var opts []grpc.ServerOption

	server := grpc.NewServer(opts...)
	provider.RegisterProviderServer(server, &HelloProviderServer{})
	server.Serve(lis)
}
