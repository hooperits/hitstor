package plugin

import (
	"context"
	"fmt"
	"io"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/scanner"
)

// FilesystemScannerPlugin implements goplugin.GRPCPlugin for the filesystem scanner type.
type FilesystemScannerPlugin struct {
	goplugin.Plugin
	// Impl is the concrete scanner.Scanner. Set by the plugin binary; nil on the host.
	Impl scanner.Scanner
}

func (p *FilesystemScannerPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterFilesystemScannerServer(s, &scannerGRPCServer{impl: p.Impl})
	return nil
}

func (p *FilesystemScannerPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &FilesystemScannerGRPCClient{client: pb.NewFilesystemScannerClient(c)}, nil
}

// --- Plugin side: gRPC server wrapping scanner.Scanner ---

// scannerGRPCServer wraps a scanner.Scanner to serve it as gRPC.
// Runs inside the plugin binary.
type scannerGRPCServer struct {
	pb.UnimplementedFilesystemScannerServer
	impl scanner.Scanner
}

func (s *scannerGRPCServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if c, ok := s.impl.(Configurable); ok {
		if err := c.Configure(req.Settings); err != nil {
			return nil, err
		}
	}
	return &pb.ConfigureResponse{}, nil
}

func (s *scannerGRPCServer) Scan(req *pb.ScanRequest, stream grpc.ServerStreamingServer[pb.ScanEntry]) error {
	ch, err := s.impl.Scan(req.Paths, req.Excludes)
	if err != nil {
		return fmt.Errorf("plugin: scan: %w", err)
	}

	for result := range ch {
		entry := &pb.ScanEntry{}
		if result.Err != nil {
			entry.ErrorMessage = result.Err.Error()
		} else {
			entry.Path = result.Path
			entry.Node = nodeToProto(result.Node)
		}
		if err := stream.Send(entry); err != nil {
			return err
		}
	}
	return nil
}

// --- Host side: gRPC client implementing scanner.Scanner ---

// FilesystemScannerGRPCClient implements scanner.Scanner by making gRPC
// calls to a plugin subprocess.
type FilesystemScannerGRPCClient struct {
	client       pb.FilesystemScannerClient
	pluginClient *goplugin.Client
}

var _ scanner.Scanner = (*FilesystemScannerGRPCClient)(nil)

func (c *FilesystemScannerGRPCClient) Scan(paths []string, excludes []string) (<-chan scanner.ScanResult, error) {
	stream, err := c.client.Scan(context.Background(), &pb.ScanRequest{
		Paths:    paths,
		Excludes: excludes,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin: scan: %w", err)
	}

	ch := make(chan scanner.ScanResult)
	go func() {
		defer close(ch)
		for {
			entry, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- scanner.ScanResult{Err: fmt.Errorf("plugin: stream: %w", err)}
				return
			}

			result := scanner.ScanResult{Path: entry.Path}
			if entry.ErrorMessage != "" {
				result.Err = fmt.Errorf("%s", entry.ErrorMessage)
			} else if entry.Node != nil {
				result.Node = protoToNode(entry.Node)
			}
			ch <- result
		}
	}()
	return ch, nil
}

func (c *FilesystemScannerGRPCClient) Close() error {
	if c.pluginClient != nil {
		c.pluginClient.Kill()
	}
	return nil
}

// --- Conversion helpers ---

func nodeToProto(n fs.Node) *pb.NodeInfo {
	return &pb.NodeInfo{
		Name:       n.Name,
		Type:       string(n.Type),
		Mode:       n.Mode,
		Uid:        n.UID,
		Gid:        n.GID,
		MtimeNs:    n.MtimeNs,
		AtimeNs:    n.AtimeNs,
		CtimeNs:    n.CtimeNs,
		Size:       n.Size,
		Inode:      n.Inode,
		Links:      n.Links,
		LinkTarget: n.LinkTarget,
	}
}

func protoToNode(ni *pb.NodeInfo) fs.Node {
	return fs.Node{
		Name:       ni.Name,
		Type:       fs.NodeType(ni.Type),
		Mode:       ni.Mode,
		UID:        ni.Uid,
		GID:        ni.Gid,
		MtimeNs:    ni.MtimeNs,
		AtimeNs:    ni.AtimeNs,
		CtimeNs:    ni.CtimeNs,
		Size:       ni.Size,
		Inode:      ni.Inode,
		Links:      ni.Links,
		LinkTarget: ni.LinkTarget,
	}
}
