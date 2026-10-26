//go:build !linux && !windows

package agent

import "fmt"

func prepareConsistencyPlatform(sess *ConsistencySession, _ []string) (*ConsistencySession, error) {
	return sess, fmt.Errorf("consistency snapshots are not supported on this platform (Windows VSS or Linux LVM/btrfs required)")
}
