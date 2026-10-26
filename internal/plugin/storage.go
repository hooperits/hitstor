package plugin

import (
	"bytes"
	"context"
	"fmt"
	"io"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/backend"
)

// StorageBackendPlugin implements goplugin.GRPCPlugin for the storage backend type.
type StorageBackendPlugin struct {
	goplugin.Plugin
	// Impl is the concrete backend.Backend. Set by the plugin binary; nil on the host.
	Impl backend.Backend
}

func (p *StorageBackendPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterStorageBackendServer(s, &storageGRPCServer{impl: p.Impl})
	return nil
}

func (p *StorageBackendPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &StorageBackendGRPCClient{client: pb.NewStorageBackendClient(c)}, nil
}

// --- Plugin side: gRPC server wrapping backend.Backend ---

// storageGRPCServer wraps a backend.Backend to serve it as gRPC.
// Runs inside the plugin binary.
type storageGRPCServer struct {
	pb.UnimplementedStorageBackendServer
	impl backend.Backend
}

func (s *storageGRPCServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if c, ok := s.impl.(Configurable); ok {
		if err := c.Configure(req.Settings); err != nil {
			return nil, err
		}
	}
	return &pb.ConfigureResponse{}, nil
}

func (s *storageGRPCServer) Save(ctx context.Context, req *pb.SaveRequest) (*pb.SaveResponse, error) {
	ft := backend.FileType(req.FileType)
	id := bytesToID(req.Id)
	if err := s.impl.Save(ctx, ft, id, bytes.NewReader(req.Data)); err != nil {
		return nil, err
	}
	return &pb.SaveResponse{}, nil
}

func (s *storageGRPCServer) Load(ctx context.Context, req *pb.LoadRequest) (*pb.LoadResponse, error) {
	ft := backend.FileType(req.FileType)
	id := bytesToID(req.Id)
	rc, err := s.impl.Load(ctx, ft, id)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return &pb.LoadResponse{Data: data}, nil
}

func (s *storageGRPCServer) Stat(ctx context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	ft := backend.FileType(req.FileType)
	id := bytesToID(req.Id)
	fi, err := s.impl.Stat(ctx, ft, id)
	if err != nil {
		return nil, err
	}
	return &pb.StatResponse{Size: fi.Size}, nil
}

func (s *storageGRPCServer) Remove(ctx context.Context, req *pb.RemoveRequest) (*pb.RemoveResponse, error) {
	ft := backend.FileType(req.FileType)
	id := bytesToID(req.Id)
	if err := s.impl.Remove(ctx, ft, id); err != nil {
		return nil, err
	}
	return &pb.RemoveResponse{}, nil
}

func (s *storageGRPCServer) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	ft := backend.FileType(req.FileType)
	ids, err := s.impl.List(ctx, ft)
	if err != nil {
		return nil, err
	}
	raw := make([][]byte, len(ids))
	for i, id := range ids {
		b := id
		raw[i] = b[:]
	}
	return &pb.ListResponse{Ids: raw}, nil
}

// --- Host side: gRPC client implementing backend.Backend ---

// StorageBackendGRPCClient implements backend.Backend by making gRPC calls
// to a plugin subprocess.
type StorageBackendGRPCClient struct {
	client       pb.StorageBackendClient
	pluginClient *goplugin.Client
}

var _ backend.Backend = (*StorageBackendGRPCClient)(nil)

func (c *StorageBackendGRPCClient) Save(ctx context.Context, ft backend.FileType, id backend.ID, rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("plugin: read save data: %w", err)
	}
	_, err = c.client.Save(ctx, &pb.SaveRequest{
		FileType: int32(ft),
		Id:       id[:],
		Data:     data,
	})
	if err != nil {
		return fmt.Errorf("plugin: save: %w", err)
	}
	return nil
}

func (c *StorageBackendGRPCClient) Load(ctx context.Context, ft backend.FileType, id backend.ID) (io.ReadCloser, error) {
	resp, err := c.client.Load(ctx, &pb.LoadRequest{
		FileType: int32(ft),
		Id:       id[:],
	})
	if err != nil {
		return nil, fmt.Errorf("plugin: load: %w", err)
	}
	return io.NopCloser(bytes.NewReader(resp.Data)), nil
}

func (c *StorageBackendGRPCClient) Stat(ctx context.Context, ft backend.FileType, id backend.ID) (backend.FileInfo, error) {
	resp, err := c.client.Stat(ctx, &pb.StatRequest{
		FileType: int32(ft),
		Id:       id[:],
	})
	if err != nil {
		return backend.FileInfo{}, fmt.Errorf("plugin: stat: %w", err)
	}
	return backend.FileInfo{Size: resp.Size}, nil
}

func (c *StorageBackendGRPCClient) Remove(ctx context.Context, ft backend.FileType, id backend.ID) error {
	_, err := c.client.Remove(ctx, &pb.RemoveRequest{
		FileType: int32(ft),
		Id:       id[:],
	})
	if err != nil {
		return fmt.Errorf("plugin: remove: %w", err)
	}
	return nil
}

func (c *StorageBackendGRPCClient) List(ctx context.Context, ft backend.FileType) ([]backend.ID, error) {
	resp, err := c.client.List(ctx, &pb.ListRequest{
		FileType: int32(ft),
	})
	if err != nil {
		return nil, fmt.Errorf("plugin: list: %w", err)
	}
	ids := make([]backend.ID, len(resp.Ids))
	for i, raw := range resp.Ids {
		ids[i] = bytesToID(raw)
	}
	return ids, nil
}

func (c *StorageBackendGRPCClient) Close() error {
	if c.pluginClient != nil {
		c.pluginClient.Kill()
	}
	return nil
}

func bytesToID(b []byte) backend.ID {
	var id backend.ID
	copy(id[:], b)
	return id
}
