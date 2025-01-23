package netconncancel

import (
	"context"
	"net"
)

type NetConnCancel struct {
	Cancel context.CancelFunc
	net.Conn
}

func New(netConn net.Conn, cancel context.CancelFunc) *NetConnCancel {
	return &NetConnCancel{
		Cancel: cancel,
		Conn:   netConn,
	}
}

func (ncc *NetConnCancel) Close() error {
	ncc.Cancel()

	return ncc.Conn.Close()
}
