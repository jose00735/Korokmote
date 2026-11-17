package server_test

import "fmt"

func fmtSscanf(s string, x, y *int) (int, error) { return fmt.Sscanf(s, "move %d %d", x, y) }
