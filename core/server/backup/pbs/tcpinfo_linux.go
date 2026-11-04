//go:build linux

package pbs

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// tcpInfo is the part of the kernel's TCP_INFO that tells a slow or lossy link
// from a refused or silent peer. Times are microseconds as the kernel reports
// them, except the last_* fields, which are milliseconds.
type tcpInfo struct {
	State          uint8  `json:"state"`
	RTTus          uint32 `json:"rtt_us"`
	RTTVarUS       uint32 `json:"rttvar_us"`
	Retransmits    uint8  `json:"retransmits"`
	TotalRetrans   uint32 `json:"total_retrans"`
	Lost           uint32 `json:"lost"`
	Unacked        uint32 `json:"unacked"`
	SndCwnd        uint32 `json:"snd_cwnd"`
	PMTU           uint32 `json:"pmtu"`
	LastDataSentMS uint32 `json:"last_data_sent_ms"`
	LastDataRecvMS uint32 `json:"last_data_recv_ms"`
	LastAckRecvMS  uint32 `json:"last_ack_recv_ms"`
}

// readTCPInfo returns the socket's TCP_INFO, or nil when c is not a TCP
// socket or the kernel refuses.
func readTCPInfo(c net.Conn) *tcpInfo {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return nil
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return nil
	}
	var info *unix.TCPInfo
	ctlErr := raw.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	})
	if ctlErr != nil || err != nil || info == nil {
		return nil
	}
	return &tcpInfo{
		State:          info.State,
		RTTus:          info.Rtt,
		RTTVarUS:       info.Rttvar,
		Retransmits:    info.Retransmits,
		TotalRetrans:   info.Total_retrans,
		Lost:           info.Lost,
		Unacked:        info.Unacked,
		SndCwnd:        info.Snd_cwnd,
		PMTU:           info.Pmtu,
		LastDataSentMS: info.Last_data_sent,
		LastDataRecvMS: info.Last_data_recv,
		LastAckRecvMS:  info.Last_ack_recv,
	}
}
