package utils

import (
	"context"
	"fmt"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	provider_grpc "github.com/lohmander/hostanapp/provider"
)

type ProviderClient struct {
	hostanv1.Provider
}

func (client *ProviderClient) GetGRPCClient() (provider_grpc.ProviderClient, error) {
	return provider_grpc.NewClient(client.Spec.URL)
}

func (client *ProviderClient) Provision(appName string, config map[string]string) (map[string]string, map[string]string, error) {
	c, err := client.GetGRPCClient()

	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	res, err := c.ProvisionAppConfig(ctx, &provider_grpc.ProvisionAppConfigRequest{
		AppName: appName,
		Config:  config,
	})

	if err != nil {
		return nil, nil, err
	}

	vars := map[string]string{}
	secrets := map[string]string{}

	for _, v := range res.ConfigVariables {
		if v.Secret {
			secrets[v.Name] = v.Value
		} else {
			vars[v.Name] = v.Value
		}
	}

	return vars, secrets, err
}

func (client *ProviderClient) Deprovision(appName string) error {
	c, err := client.GetGRPCClient()

	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = c.DeprovisionAppConfig(ctx, &provider_grpc.DeprovisionAppConfigRequest{
		AppName: appName,
	})

	return err
}

type ProviderClientSet struct {
	hostanv1.ProviderList
}

func (clientSet *ProviderClientSet) Get(name string) (*ProviderClient, error) {
	for _, provider := range clientSet.Items {
		if provider.Name == name {
			return &ProviderClient{Provider: provider}, nil
		}
	}

	return nil, fmt.Errorf("Could not find a provider with name %s", name)
}
