//go:build !windows

package agent

import (
	"os"
	"path/filepath"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// listRootDirs returns common root-level directories on Linux.
func (s *AgentServer) listRootDirs() (*pb.ListDirectoryResponse, error) {
	roots := []string{"/", "/boot", "/etc", "/home", "/opt", "/root", "/srv", "/var"}
	var result []*pb.DirEntry

	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}
		entry := &pb.DirEntry{
			Name:        filepath.Base(root),
			Path:        root,
			IsDir:       true,
			Mode:        uint32(info.Mode()),
			Readable:    isReadable(root),
			HasChildren: hasSubdirs(root),
		}
		if root == "/" {
			entry.Name = "/"
		}
		result = append(result, entry)
	}

	return &pb.ListDirectoryResponse{Entries: result}, nil
}
