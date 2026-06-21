package ham

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ReadKISSPort reads the KISSPORT value from a Direwolf config file.
// Returns 8001 if not found.
func ReadKISSPort(configPath string) (int, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return 0, fmt.Errorf("opening config %q: %w", configPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToUpper(line), "KISSPORT") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				port, err := strconv.Atoi(fields[1])
				if err != nil {
					return 0, fmt.Errorf("invalid KISSPORT %q: %w", fields[1], err)
				}
				return port, nil
			}
		}
	}
	return 8001, nil // default
}
