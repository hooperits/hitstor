//go:build windows

package agent

import (
	"fmt"
	"os"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"golang.org/x/sys/windows"
)

// listRootDirs enumerates mounted drive letters on Windows using the
// GetLogicalDrives syscall for reliable detection.
func (s *AgentServer) listRootDirs() (*pb.ListDirectoryResponse, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("GetLogicalDrives: %w", err)
	}

	var result []*pb.DirEntry
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		drive := fmt.Sprintf("%c:\\", 'A'+i)
		info, err := os.Stat(drive)
		if err != nil {
			continue
		}
		result = append(result, &pb.DirEntry{
			Name:        drive,
			Path:        drive,
			IsDir:       true,
			Mode:        uint32(info.Mode()),
			Readable:    isReadable(drive),
			HasChildren: true, // drive roots always have children
		})
	}

	return &pb.ListDirectoryResponse{Entries: result}, nil
}
