// packages/tpd/connection.go

package tpd

import (
	"fmt"
	"net"

	logger "pegasus_suite/logger"
)

// kernel drops datagrams silently once recieve buffer is full
// very large buffer to be safe
const socketBufferSize = 4 * 1024 * 1024

func listenUDP(port string) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp4", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("tpd: resolve :%s: %w", port, err)
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("tpd: listen on :%s: %w", port, err)
	}

	if err := conn.SetReadBuffer(socketBufferSize); err != nil {
		logger.Warn(logger.ErrorLog{
			Message: fmt.Sprintf("tpd could not size the udp receive buffer; bursts may be dropped error=%v", err),
		})
	}

	return conn, nil
}
