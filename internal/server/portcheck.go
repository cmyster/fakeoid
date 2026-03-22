package server

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// killProcessOnPort kills whatever is listening on the given TCP port.
func killProcessOnPort(port int) error {
	return killProcessOnPortWithFinder(port, readProcNetTCP, findInodePIDs, killPID)
}

// killProcessOnPortWithFinder is the fully injectable version for testing.
func killProcessOnPortWithFinder(port int, readProc func() ([]byte, error), pidFinder func() (map[int]int, error), killer func(int) error) error {
	data, err := readProc()
	if err != nil {
		return nil // gracefully handle inability to read /proc
	}

	hexPort := fmt.Sprintf("%04X", port)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(parts[1], hexPort) {
			inode, err := strconv.Atoi(fields[9])
			if err != nil {
				continue
			}
			pidMap, err := pidFinder()
			if err != nil {
				return nil
			}
			if pid, ok := pidMap[inode]; ok {
				return killer(pid)
			}
		}
	}
	return nil
}

func readProcNetTCP() ([]byte, error) {
	return os.ReadFile("/proc/net/tcp")
}

func findInodePIDs() (map[int]int, error) {
	result := make(map[int]int)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") {
				inodeStr := strings.TrimPrefix(link, "socket:[")
				inodeStr = strings.TrimSuffix(inodeStr, "]")
				inode, err := strconv.Atoi(inodeStr)
				if err != nil {
					continue
				}
				result[inode] = pid
			}
		}
	}
	return result, nil
}

func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
