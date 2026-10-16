package worktrees

import (
	"fmt"
	"net"
	"slices"
)

// The ports tasks lease for their dev servers. With one shared port, two of
// three agents' tests hit another agent's server (proof 06).
const (
	FirstPort = 7900
	LastPort  = 7999
)

// LeasePort picks the lowest port in the range that no unfinished task holds
// and nothing is listening on right now.
func LeasePort(taken []int) (int, error) {
	for port := FirstPort; port <= LastPort; port++ {
		if !slices.Contains(taken, port) && isFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port between %d and %d", FirstPort, LastPort)
}

func isFree(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
