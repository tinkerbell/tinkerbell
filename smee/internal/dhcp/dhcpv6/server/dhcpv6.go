// Package server provides DHCPv6 listening and serving functionality.
package server

import (
	"context"
	"errors"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/server6"
)

const (
	// DefaultHandlerTimeout is the maximum time a DHCPv6 handler gets to process one packet.
	DefaultHandlerTimeout = 10 * time.Second
	defaultQueuePerWorker = 16
	minDefaultWorkers     = 4
)

// Handler is called every time a valid DHCPv6 message is received.
type Handler interface {
	Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, msg dhcpv6.DHCPv6)
}

type handlerJob struct {
	conn    net.PacketConn
	peer    net.Addr
	msg     dhcpv6.DHCPv6
	handler Handler
}

// DHCPv6 represents a DHCPv6 server object.
type DHCPv6 struct {
	ifname string
	addr   *net.UDPAddr

	handlers []Handler
	logger   logr.Logger

	// HandlerWorkers is the number of workers that process DHCPv6 packets.
	HandlerWorkers int
	// QueueSize is the maximum number of pending handler jobs.
	QueueSize int
	// HandlerTimeout is the per-packet timeout passed to handlers.
	HandlerTimeout time.Duration

	server *server6.Server
}

// NewServer initializes and returns a new DHCPv6 server.
func NewServer(ifname string, addr *net.UDPAddr, handlers ...Handler) *DHCPv6 {
	return &DHCPv6{
		ifname:         ifname,
		addr:           addr,
		handlers:       handlers,
		logger:         logr.Discard(),
		HandlerTimeout: DefaultHandlerTimeout,
	}
}

// SetLogger configures the logger used by the DHCPv6 server.
func (s *DHCPv6) SetLogger(log logr.Logger) {
	if log.GetSink() != nil {
		s.logger = log
	}
}

// Serve serves requests until the server exits or ctx is canceled.
func (s *DHCPv6) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.setDefaults()
	jobs := make(chan handlerJob, s.QueueSize)

	srv, err := server6.NewServer(
		s.ifname,
		s.addr,
		func(conn net.PacketConn, peer net.Addr, msg dhcpv6.DHCPv6) {
			for _, handler := range s.handlers {
				s.enqueue(ctx, jobs, handlerJob{
					conn:    conn,
					peer:    peer,
					msg:     msg,
					handler: handler,
				})
			}
		},
	)
	if err != nil {
		return err
	}

	var workerWg sync.WaitGroup
	s.startWorkers(ctx, jobs, &workerWg)

	s.server = srv
	defer func() {
		_ = srv.Close()
		workerWg.Wait()
		s.server = nil
	}()

	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Serve()
	}()

	select {
	case <-ctx.Done():
		closeErr := srv.Close()
		serveErr := <-errCh

		if serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
			return serveErr
		}
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			return closeErr
		}

		return nil

	case err := <-errCh:
		cancel()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func (s *DHCPv6) setDefaults() {
	if s.HandlerWorkers <= 0 {
		s.HandlerWorkers = max(runtime.GOMAXPROCS(0), minDefaultWorkers)
	}
	if s.QueueSize <= 0 {
		s.QueueSize = s.HandlerWorkers * defaultQueuePerWorker
	}
}

func (s *DHCPv6) startWorkers(ctx context.Context, jobs <-chan handlerJob, wg *sync.WaitGroup) {
	for range s.HandlerWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					s.handle(ctx, job)
				}
			}
		}()
	}
}

func (s *DHCPv6) enqueue(ctx context.Context, jobs chan<- handlerJob, job handlerJob) bool {
	select {
	case <-ctx.Done():
		return false
	case jobs <- job:
		return true
	default:
		s.logger.Info("dropping DHCPv6 packet: handler queue full", "peer", job.peer, "queueSize", cap(jobs), "handlerWorkers", s.HandlerWorkers)
		return false
	}
}

func (s *DHCPv6) handle(ctx context.Context, job handlerJob) {
	if job.handler == nil {
		return
	}
	if s.HandlerTimeout <= 0 {
		job.handler.Handle(ctx, job.conn, job.peer, job.msg)
		return
	}

	handlerCtx, cancel := context.WithTimeout(ctx, s.HandlerTimeout)
	defer cancel()
	job.handler.Handle(handlerCtx, job.conn, job.peer, job.msg)
}

// Close sends a termination request to the server and closes the UDP listener.
func (s *DHCPv6) Close() error {
	if s.server == nil {
		return nil
	}

	return s.server.Close()
}
