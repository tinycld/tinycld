//go:build !linux

package pbs

import "net"

// tcpInfo is empty off Linux: TCP_INFO is read only there, where the router
// and the backups run.
type tcpInfo struct{}

func readTCPInfo(net.Conn) *tcpInfo { return nil }
