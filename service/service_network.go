package service

import (
	"context"
	"math/big"

	"github.com/coinbase/rosetta-sdk-go/server"
	"github.com/coinbase/rosetta-sdk-go/types"

	"github.com/figment-networks/avalanche-rosetta/client"
	"github.com/figment-networks/avalanche-rosetta/mapper"
)

// NetworkService implements all /network endpoints
type NetworkService struct {
	config *Config
	client client.Client
}

// NewNetworkService returns a new network servicer
func NewNetworkService(config *Config, client client.Client) server.NetworkAPIServicer {
	return &NetworkService{
		config: config,
		client: client,
	}
}

// NetworkList implements the /network/list endpoint
func (s *NetworkService) NetworkList(ctx context.Context, request *types.MetadataRequest) (*types.NetworkListResponse, *types.Error) {
	return &types.NetworkListResponse{
		NetworkIdentifiers: []*types.NetworkIdentifier{
			s.config.NetworkID,
		},
	}, nil
}

// NetworkStatus implements the /network/status endpoint
func (s *NetworkService) NetworkStatus(ctx context.Context, request *types.NetworkRequest) (*types.NetworkStatusResponse, *types.Error) {
	if s.config.IsOfflineMode() {
		return nil, errUnavailableOffline
	}

	// Fetch the latest block
	blockHeader, err := s.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, wrapError(errClientError, err)
	}
	if blockHeader == nil {
		return nil, wrapError(errClientError, "latest block not found")
	}

	// Fetch the genesis block
	genesisHeader, err := s.client.HeaderByNumber(ctx, big.NewInt(0))
	if err != nil {
		return nil, wrapError(errClientError, err)
	}
	if genesisHeader == nil {
		return nil, wrapError(errClientError, "genesis block not found")
	}

	// Fetch peers
	infoPeers, err := s.client.Peers(ctx)
	if err != nil {
		return nil, wrapError(errClientError, err)
	}
	peers := mapper.Peers(infoPeers)

	return &types.NetworkStatusResponse{
		CurrentBlockTimestamp: int64(blockHeader.Time * 1000),
		CurrentBlockIdentifier: &types.BlockIdentifier{
			Index: blockHeader.Number.Int64(),
			Hash:  blockHeader.Hash().String(),
		},
		GenesisBlockIdentifier: &types.BlockIdentifier{
			Index: genesisHeader.Number.Int64(),
			Hash:  genesisHeader.Hash().String(),
		},
		SyncStatus: &types.SyncStatus{
			Synced: types.Bool(true),
		},
		Peers: peers,
	}, nil
}

// NetworkOptions implements the /network/options endpoint
func (s *NetworkService) NetworkOptions(ctx context.Context, request *types.NetworkRequest) (*types.NetworkOptionsResponse, *types.Error) {
	if s.config.IsOfflineMode() {
		return nil, errUnavailableOffline
	}

	nodeVersion, err := s.client.NodeVersion(ctx)
	if err != nil {
		return nil, wrapError(errClientError, nodeVersion)
	}

	middlewareVersion := MiddlewareVersion

	resp := &types.NetworkOptionsResponse{
		Version: &types.Version{
			RosettaVersion:    types.RosettaAPIVersion,
			MiddlewareVersion: &middlewareVersion,
			NodeVersion:       nodeVersion,
		},
		Allow: &types.Allow{
			OperationStatuses:       mapper.OperationStatuses,
			OperationTypes:          mapper.OperationTypes,
			CallMethods:             mapper.CallMethods,
			Errors:                  Errors,
			HistoricalBalanceLookup: true,
		},
	}

	return resp, nil
}
