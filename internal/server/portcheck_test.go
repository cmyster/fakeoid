package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKillProcessOnPortUnused(t *testing.T) {
	// /proc/net/tcp with only port 80 (0x0050), searching for 9999
	reader := func() ([]byte, error) {
		return []byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
`), nil
	}

	pidFinder := func() (map[int]int, error) {
		return map[int]int{12345: 999}, nil
	}

	var killed bool
	err := killProcessOnPortWithFinder(9999, reader, pidFinder, func(pid int) error {
		killed = true
		return nil
	})
	assert.NoError(t, err)
	assert.False(t, killed, "should not kill anything on unused port")
}

func TestKillProcessOnPortFindsAndKills(t *testing.T) {
	// Port 8080 = 0x1F90
	reader := func() ([]byte, error) {
		return []byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 99999 1 0000000000000000 100 0 0 10 0
`), nil
	}

	pidFinder := func() (map[int]int, error) {
		return map[int]int{99999: 1234}, nil
	}

	var killedPID int
	err := killProcessOnPortWithFinder(8080, reader, pidFinder, func(pid int) error {
		killedPID = pid
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1234, killedPID)
}

func TestKillProcessOnPortHexParsing(t *testing.T) {
	tests := []struct {
		port    int
		hexPort string
	}{
		{80, "0050"},
		{8080, "1F90"},
		{443, "01BB"},
		{3000, "0BB8"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("port_%d", tt.port), func(t *testing.T) {
			reader := func() ([]byte, error) {
				return []byte(fmt.Sprintf(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:%s 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 55555 1 0000000000000000 100 0 0 10 0
`, tt.hexPort)), nil
			}

			pidFinder := func() (map[int]int, error) {
				return map[int]int{55555: 42}, nil
			}

			var killed bool
			err := killProcessOnPortWithFinder(tt.port, reader, pidFinder, func(pid int) error {
				killed = true
				return nil
			})
			assert.NoError(t, err)
			assert.True(t, killed, "should kill process on port %d", tt.port)
		})
	}
}

func TestKillProcessOnPortProcReadError(t *testing.T) {
	reader := func() ([]byte, error) {
		return nil, fmt.Errorf("permission denied")
	}
	pidFinder := func() (map[int]int, error) {
		return nil, nil
	}

	err := killProcessOnPortWithFinder(8080, reader, pidFinder, func(pid int) error {
		t.Fatal("should not try to kill")
		return nil
	})
	// Should return nil on read error (best-effort)
	assert.NoError(t, err)
}

func TestKillProcessOnPortNoInodeMatch(t *testing.T) {
	// Port matches but no PID found for inode
	reader := func() ([]byte, error) {
		return []byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 99999 1 0000000000000000 100 0 0 10 0
`), nil
	}

	pidFinder := func() (map[int]int, error) {
		// No matching inode
		return map[int]int{11111: 1234}, nil
	}

	var killed bool
	err := killProcessOnPortWithFinder(8080, reader, pidFinder, func(pid int) error {
		killed = true
		return nil
	})
	assert.NoError(t, err)
	assert.False(t, killed, "should not kill when no PID matches inode")
}
