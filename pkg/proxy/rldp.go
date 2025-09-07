package proxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/rldp"
)

const maxChunkSize = 128 << 10
const maxAnswerSize = 16 << 10

type rldpReader struct {
	client  *rldp.RLDP
	queryId []byte
	seqno   int32
	last    bool
	ctx     context.Context
}

func (r *rldpReader) Read(p []byte) (n int, err error) {
	if r == nil || r.last {
		return 0, io.EOF
	}

	for len(p) > 0 {
		chunkSize := int32(min(len(p), maxChunkSize))
		data, last, err := r.getNextPart(chunkSize)
		if err != nil {
			return 0, err
		}
		copied := copy(p, data)
		n += copied
		r.seqno++
		if last {
			r.last = true
			return n, io.EOF
		}
		p = p[copied:]
	}
	return n, nil
}

func (r *rldpReader) WriteTo(w io.Writer) (n int64, err error) {
	if r == nil || r.last {
		return 0, nil
	}

	for !r.last {
		data, last, err := r.getNextPart(maxChunkSize)
		if err != nil {
			return n, err
		}
		written, err := w.Write(data)
		n += int64(written)
		if err != nil {
			return n, err
		}
		r.seqno++
		r.last = last
	}
	return n, nil
}

func (r *rldpReader) getNextPart(chunkSize int32) ([]byte, bool, error) {
	req := GetNextPayloadPart{
		Id:           r.queryId,
		Seqno:        r.seqno,
		MaxChunkSize: chunkSize,
	}
	var part PayloadPart
	err := r.client.DoQuery(r.ctx, uint64(maxAnswerSize+chunkSize), req, &part)
	if err != nil {
		return nil, false, err
	}
	if int32(len(part.Data)) > chunkSize {
		return nil, false, fmt.Errorf("client sent too much data %d > %d", len(part.Data), chunkSize)
	}
	return part.Data, part.Last, nil
}

type RLDPConnector struct {
	gate  *adnl.Gateway
	dht   *dht.Client
	conns map[string]*RLDPConnection
	mx    sync.RWMutex
}

func NewRLDPConnector(gate *adnl.Gateway, dht *dht.Client) *RLDPConnector {
	return &RLDPConnector{
		gate:  gate,
		dht:   dht,
		conns: make(map[string]*RLDPConnection),
	}
}

func (c *RLDPConnector) GetConnection(ctx context.Context, id []byte) (*RLDPConnection, error) {
	addresses, pubKey, err := c.dht.FindAddresses(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to find address of %x in DHT: %w", id, err)
	}
	if len(addresses.Addresses) == 0 {
		return nil, fmt.Errorf("no addresses found for %x", id)
	}
	clientId := string(pubKey)

	c.mx.RLock()
	conn := c.conns[clientId]
	c.mx.RUnlock()
	if conn != nil {
		return conn, nil
	}

	c.mx.Lock()
	defer c.mx.Unlock()
	conn = c.conns[clientId]
	if conn != nil {
		return conn, nil
	}

	for _, udp := range addresses.Addresses {
		addr := fmt.Sprintf("%s:%d", udp.IP.String(), udp.Port)
		peer, err := c.gate.RegisterClient(addr, pubKey)
		if err != nil {
			continue
		}
		client := rldp.NewClientV2(peer)
		conn := &RLDPConnection{
			client: client,
		}
		client.SetOnQuery(conn.handleQuery)
		peer.SetDisconnectHandler(c.removeClient)
		c.conns[clientId] = conn
		return conn, nil
	}
	return nil, fmt.Errorf("unable to connect to %x", id)
}

func (c *RLDPConnector) removeClient(addr string, pubKey ed25519.PublicKey) {
	c.mx.Lock()
	defer c.mx.Unlock()
	id := string(pubKey)
	conn := c.conns[id]
	conn.Close()
	delete(c.conns, id)
}

type RLDPConnection struct {
	client   *rldp.RLDP
	requests map[string]io.Reader
	mx       sync.RWMutex
}

func (c *RLDPConnection) SendRequest(ctx context.Context, req *Request, payload io.Reader) (*Response, io.Reader, error) {
	if req.Id == nil {
		req.Id = make([]byte, 32)
		rand.Read(req.Id)
	}
	if payload != nil {
		c.mx.Lock()
		if c.requests == nil {
			c.requests = make(map[string]io.Reader)
		}
		c.requests[string(req.Id)] = payload
		c.mx.Unlock()
		// i guess that upstream should read whole payload by the time DoQuery returns
		// so we defer discard in case the upstream doesn't read whole payload
		defer c.discardRequest(req.Id)
	}
	resp := &Response{}
	err := c.client.DoQuery(ctx, maxAnswerSize, req, resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.NoPayload {
		return resp, nil, nil
	}
	reader := &rldpReader{
		client:  c.client,
		queryId: req.Id,
		ctx:     ctx,
	}
	return resp, reader, nil
}

func (c *RLDPConnection) Close() {
	c.mx.Lock()
	for k := range c.requests {
		delete(c.requests, k)
	}
	c.mx.Unlock()
	c.client.Close()
}

func (c *RLDPConnection) discardRequest(id []byte) {
	c.mx.Lock()
	defer c.mx.Unlock()
	delete(c.requests, string(id))
}

func (c *RLDPConnection) handleQuery(transferId []byte, query *rldp.Query) error {
	switch data := query.Data.(type) {
	case GetNextPayloadPart:
		c.mx.RLock()
		var request io.Reader
		if c.requests != nil {
			request = c.requests[string(data.Id)]
		}
		c.mx.RUnlock()
		if request == nil {
			return fmt.Errorf("no active request %x found", data.Id)
		}

		var last bool
		part := make([]byte, data.MaxChunkSize)
		n, err := request.Read(part)
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("failed to read from active request %x: %w", data.Id, err)
			}
			last = true
		}
		ctx := context.Background()
		payload := PayloadPart{
			Data:    part[:n],
			Trailer: nil,
			Last:    last,
		}
		err = c.client.SendAnswer(ctx, query.MaxAnswerSize, query.Timeout, query.ID, transferId, payload)
		if err != nil {
			return fmt.Errorf("failed to send payload part answer: %w", err)
		}
		if last {
			c.discardRequest(data.Id)
		}
		return nil
	}
	return fmt.Errorf("bogus query type: %s", reflect.TypeOf(query.Data))
}
