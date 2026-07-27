package syslog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

var syslogMessagePool = sync.Pool{
	New: func() interface{} { return new(message) },
}

type Receiver struct {
	conn   *net.UDPConn
	msgCh  chan *message
	done   chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	err    error
	Logger logr.Logger
}

func StartReceiver(ctx context.Context, logger logr.Logger, laddr string, parsers int) (*Receiver, error) {
	if parsers < 1 {
		parsers = 1
	}

	addr, err := net.ResolveUDPAddr("udp4", laddr)
	if err != nil {
		return nil, fmt.Errorf("resolve syslog udp listen address: %w", err)
	}

	c, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on syslog udp address: %w", err)
	}

	r := &Receiver{
		conn:   c,
		msgCh:  make(chan *message, parsers*64),
		done:   make(chan struct{}),
		Logger: logger,
	}

	r.wg.Add(parsers)
	for i := 0; i < parsers; i++ {
		go r.runParser()
	}
	go r.run(ctx)

	return r, nil
}

func (r *Receiver) Done() <-chan struct{} {
	return r.done
}

func (r *Receiver) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *Receiver) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *Receiver) shutdown() {
	r.conn.Close()
	close(r.msgCh)
	r.wg.Wait()
	close(r.done)
}

func (r *Receiver) run(ctx context.Context) {
	var msg *message
	defer func() {
		if msg != nil {
			syslogMessagePool.Put(msg)
		}
	}()

	go func() {
		<-ctx.Done()
		r.conn.Close()
	}()

	for {
		if msg == nil {
			var ok bool
			msg, ok = syslogMessagePool.Get().(*message)
			if !ok {
				r.Logger.Error(errors.New("error type asserting pool item into message"), "error type asserting pool item into message")

				continue
			}
		}
		n, from, err := r.conn.ReadFromUDP(msg.buf[:])
		if err != nil {
			if ctx.Err() != nil {
				r.shutdown()
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) {
				r.Logger.Error(err, "error reading udp message")
				continue
			}
			r.setErr(fmt.Errorf("error reading udp message: %w", err))
			r.shutdown()
			return
		}
		msg.time = time.Now().UTC()
		msg.host = from.IP
		msg.size = n
		r.msgCh <- msg
		msg = nil
	}
}

func toStructured(m *message) map[string]interface{} {
	structured := make(map[string]interface{})
	if m.Facility().String() != "" {
		structured["facility"] = m.Facility().String()
	}
	if m.Severity().String() != "" {
		structured["severity"] = m.Severity().String()
	}
	if len(m.hostname) > 0 {
		structured["hostname"] = string(m.hostname)
	}
	if len(m.app) > 0 {
		structured["app-name"] = string(m.app)
	}
	if len(m.procid) > 0 {
		structured["procid"] = string(m.procid)
	}
	if len(m.msgid) > 0 {
		structured["msgid"] = string(m.msgid)
	}
	if len(m.msg) > 0 {
		if m.msg[0] == '{' {
			var j map[string]interface{}
			if err := json.Unmarshal(m.msg, &j); err == nil {
				structured["msg"] = j
			} else {
				structured["msg"] = string(m.msg)
			}
		} else {
			structured["msg"] = string(m.msg)
		}
	}
	structured["host"] = m.host.String()

	return structured
}

func (r *Receiver) runParser() {
	defer r.wg.Done()
	for m := range r.msgCh {
		if m.parse() {
			structured := toStructured(m)
			if m.Severity() == DEBUG {
				r.Logger.V(1).Info("syslog message received", "syslog", structured)
			} else {
				r.Logger.Info("syslog message received", "syslog", structured)
			}
		} else {
			r.Logger.V(1).Info("unparseable syslog message", "raw", m)
		}
		m.reset()
		syslogMessagePool.Put(m)
	}
}
