package e2etest

import (
	"fmt"
	mrand "math/rand/v2"
	"net"
	"os"
	"sync"
	"testing"
)

// Track allocated ports, protected by a mutex
var (
	allocatedPorts = make(map[int]struct{})
	portMutex      sync.Mutex
)

// AllocateUniquePort tries to find an available TCP port on the localhost
// by testing multiple random ports within a specified range.
func AllocateUniquePort(t *testing.T) (int, string) {
	randPort := func(base, spread int) int {
		return base + mrand.IntN(spread)
	}

	// Base port and spread range for port selection
	const (
		basePort  = 40000
		portRange = 50000
	)

	var url string
	// Try up to 10 times to find an available port
	for i := 0; i < 30; i++ {
		port := randPort(basePort, portRange)

		// Lock the mutex to check and modify the shared map
		portMutex.Lock()
		if _, exists := allocatedPorts[port]; exists {
			// Port already allocated, try another one
			portMutex.Unlock()
			continue
		}

		url = fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", url)
		if err != nil {
			portMutex.Unlock()
			continue
		}

		allocatedPorts[port] = struct{}{}
		portMutex.Unlock()

		if err := listener.Close(); err != nil {
			continue
		}

		return port, url
	}

	// If no available port was found, fail the test
	t.Fatalf("failed to find an available port in range %d-%d", basePort, basePort+portRange)
	return 0, ""
}

func baseDir(pattern string) (string, error) {
	tempPath := os.TempDir()

	tempName, err := os.MkdirTemp(tempPath, pattern)
	if err != nil {
		return "", err
	}

	err = os.Chmod(tempName, 0755)

	if err != nil {
		return "", err
	}

	return tempName, nil
}
