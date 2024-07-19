package utils

import (
	"context"
	"sync"

	"google.golang.org/grpc"
)

type ConnPool struct {
	mu        sync.Mutex
	conns     []*grpc.ClientConn
	factory   func() (*grpc.ClientConn, error)
	size      int
	available int
}

func NewConnPool(factory func() (*grpc.ClientConn, error), size int) *ConnPool {
	return &ConnPool{
		factory:   factory,
		size:      size,
		available: size,
		conns:     make([]*grpc.ClientConn, 0, size),
	}
}

func (p *ConnPool) Get(ctx context.Context) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) > 0 {
		conn := p.conns[len(p.conns)-1]
		p.conns = p.conns[:len(p.conns)-1]
		p.available--
		return conn, nil
	}

	if p.available > 0 {
		conn, err := p.factory()
		if err != nil {
			return nil, err
		}
		p.available--
		return conn, nil
	}

	// Wait for a connection to become available
	for {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			return nil, ctx.Err()
		default:
			p.mu.Lock()
			if len(p.conns) > 0 {
				conn := p.conns[len(p.conns)-1]
				p.conns = p.conns[:len(p.conns)-1]
				p.available--
				return conn, nil
			}
			p.mu.Unlock()
		}
		p.mu.Lock()
	}
}

func (p *ConnPool) Put(conn *grpc.ClientConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) < p.size {
		p.conns = append(p.conns, conn)
		p.available++
	} else {
		conn.Close()
	}
}

func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = nil
	p.available = 0
}
