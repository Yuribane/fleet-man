package portforward

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// startProxy opens a TCP listener on localPort and proxies each accepted
// connection to hostname:remotePort. All goroutines stop when the
// listener is closed.
func startProxy(localPort int, hostname string, remotePort int) (*Forward, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", localPort))
	if err != nil {
		return nil, err
	}

	target := net.JoinHostPort(hostname, fmt.Sprintf("%d", remotePort))
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go proxy(conn, target)
		}
	}()

	return &Forward{
		LocalPort:  localPort,
		RemotePort: remotePort,
		listener:   ln,
		done:       done,
	}, nil
}

// proxy pipes data between a local connection and a remote target.
func proxy(local net.Conn, target string) {
	remote, err := net.Dial("tcp", target)
	if err != nil {
		local.Close()
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(remote, local)
		if tcpConn, ok := remote.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		io.Copy(local, remote)
		if tcpConn, ok := local.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
	local.Close()
	remote.Close()
}
