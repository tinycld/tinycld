package subscriptions

import (
	"bytes"
	"strconv"
	"sync"
)

// Resumable clients.
//
// A DefaultClient lives exactly as long as its SSE request, so every event
// sent while the connection is down is lost and the subscriber must refetch
// everything. A ResumableClient outlives its connection: its messages go
// into a FIFO, numbered in order, and a connection (a ResumableConn) only
// drains that FIFO. When the connection ends, the FIFO keeps filling until
// a new connection resumes the client or the owner discards it.
//
// Nothing is kept once it is handed to a connection, so the FIFO is near
// empty while a connection is attached. A message handed to a connection
// that then died may never have reached the subscriber; the subscriber's
// last received seq tells, and Attach refuses a resume that would skip it.

// ResumableOptions configures a ResumableClient.
type ResumableOptions struct {
	// MaxMessages and MaxBytes bound the FIFO. A client whose FIFO
	// exceeds either can no longer replay every message and overflows.
	MaxMessages int
	MaxBytes    int

	// Budget bounds the FIFO bytes of all detached clients that share it.
	// A detached client that cannot reserve its bytes overflows. Nil means
	// no shared bound.
	Budget *QueueBudget

	// OnOverflow is called, outside the client lock, when the client
	// overflows. It should unregister the client from its broker. Nil
	// discards the client.
	OnOverflow func(client *ResumableClient)
}

type queuedMessage struct {
	seq uint64
	msg Message
}

type attachment struct {
	gen      uint64
	out      chan Message
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// ResumableClient is a Client that keeps its subscriptions, its store and
// its undelivered messages across connections.
//
// Its own Channel is never written to: read a connection's channel instead
// (see Attach).
type ResumableClient struct {
	*DefaultClient

	options ResumableOptions

	mu       sync.Mutex
	queue    []queuedMessage
	bytes    int
	reserved int
	nextSeq  uint64
	lastSent uint64
	gen      uint64
	att      *attachment
	notify   chan struct{}
	gone     bool
}

// ensures that ResumableClient satisfies the Client interface
var _ Client = (*ResumableClient)(nil)

// NewResumableClient creates a new detached ResumableClient.
func NewResumableClient(options ResumableOptions) *ResumableClient {
	return &ResumableClient{
		DefaultClient: NewDefaultClient(),
		options:       options,
		notify:        make(chan struct{}, 1),
	}
}

// LastSent returns the seq of the last message handed to a connection.
func (c *ResumableClient) LastSent() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastSent
}

// Send implements the [Client.Send] interface method.
//
// It never blocks: the message is numbered and queued. A JSON object
// payload gets the number as a top-level "seq" field.
func (c *ResumableClient) Send(m Message) {
	c.mu.Lock()

	if c.gone {
		c.mu.Unlock()
		return
	}

	c.nextSeq++
	m.Data = withSeq(m.Data, c.nextSeq)
	c.queue = append(c.queue, queuedMessage{seq: c.nextSeq, msg: m})
	c.bytes += len(m.Data)

	overflow := c.exceedsLimits()
	if !overflow && c.att == nil {
		overflow = !c.reserve(len(m.Data))
	}

	select {
	case c.notify <- struct{}{}:
	default:
	}

	c.mu.Unlock()

	if overflow {
		c.overflow()
	}
}

// Attach starts a new connection that delivers the queued messages, in
// order, followed by every later message.
//
// after is the seq of the last message the subscriber received (0 if none).
// A resume is refused (ok is false) when the client is discarded or when a
// message after `after` was already handed to a connection and is no longer
// queued.
//
// A connection that is still attached is ended first: its channel closes,
// so its request returns. A new subscriber connecting with the client's
// credentials means the old connection is dead or abandoned.
func (c *ResumableClient) Attach(after uint64) (conn *ResumableConn, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// another Attach may win the race between ending the old connection
	// and taking the lock, so end whichever connection is attached now
	for c.att != nil {
		c.mu.Unlock()
		c.endAttachment()
		c.mu.Lock()
	}

	if c.gone || after != c.lastSent {
		return nil, false
	}

	c.gen++
	att := &attachment{
		gen:  c.gen,
		out:  make(chan Message),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	c.att = att
	c.release()

	go c.pump(att)

	return &ResumableConn{ResumableClient: c, att: att}, true
}

// Attached reports whether a connection is attached.
func (c *ResumableClient) Attached() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.att != nil
}

// Detached reports whether the client is still detached by the connection
// with the given generation, i.e. no connection attached since it ended.
func (c *ResumableClient) Detached(gen uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return !c.gone && c.att == nil && c.gen == gen
}

// Discard implements the [Client.Discard] interface method.
//
// It ends the attached connection, if any, and drops the queue.
func (c *ResumableClient) Discard() {
	c.mu.Lock()
	if c.gone {
		c.mu.Unlock()
		return
	}
	c.gone = true
	c.mu.Unlock()

	c.endAttachment()

	c.mu.Lock()
	c.queue = nil
	c.bytes = 0
	c.release()
	c.mu.Unlock()

	c.DefaultClient.Discard()
}

// IsDiscarded implements the [Client.IsDiscarded] interface method.
func (c *ResumableClient) IsDiscarded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.gone
}

// detach stops the connection with the given generation and keeps its
// undelivered messages queued. It reports false when that connection is no
// longer the attached one.
func (c *ResumableClient) detach(gen uint64) bool {
	c.mu.Lock()
	att := c.att
	if c.gone || att == nil || att.gen != gen {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	c.stopPump(att)

	c.mu.Lock()
	if c.att != att {
		c.mu.Unlock()
		return false
	}
	c.att = nil
	overflow := !c.reserve(c.bytes)
	c.mu.Unlock()

	if overflow {
		c.overflow()
		return false
	}

	return true
}

// endAttachment stops the attached connection, if any, and closes its
// channel so that its request returns.
func (c *ResumableClient) endAttachment() {
	c.mu.Lock()
	att := c.att
	c.att = nil
	c.mu.Unlock()

	if att == nil {
		return
	}

	c.stopPump(att)
	close(att.out)
}

func (c *ResumableClient) stopPump(att *attachment) {
	att.stopOnce.Do(func() { close(att.stop) })
	<-att.done
}

// pump hands the queued messages, one at a time, to the connection's
// channel until the attachment stops. A message the connection did not take
// goes back to the head of the queue.
func (c *ResumableClient) pump(att *attachment) {
	defer close(att.done)

	for {
		c.mu.Lock()
		if len(c.queue) == 0 {
			c.mu.Unlock()
			select {
			case <-c.notify:
				continue
			case <-att.stop:
				return
			}
		}
		head := c.queue[0]
		c.queue = c.queue[1:]
		c.bytes -= len(head.msg.Data)
		c.mu.Unlock()

		select {
		case att.out <- head.msg:
			c.mu.Lock()
			c.lastSent = head.seq
			c.mu.Unlock()
		case <-att.stop:
			c.mu.Lock()
			if !c.gone {
				c.queue = append([]queuedMessage{head}, c.queue...)
				c.bytes += len(head.msg.Data)
			}
			c.mu.Unlock()
			return
		}
	}
}

// exceedsLimits must be called with the lock held.
func (c *ResumableClient) exceedsLimits() bool {
	return (c.options.MaxMessages > 0 && len(c.queue) > c.options.MaxMessages) ||
		(c.options.MaxBytes > 0 && c.bytes > c.options.MaxBytes)
}

// reserve must be called with the lock held.
func (c *ResumableClient) reserve(n int) bool {
	if c.options.Budget == nil || n == 0 {
		return true
	}
	if !c.options.Budget.reserve(n) {
		return false
	}
	c.reserved += n
	return true
}

// release must be called with the lock held.
func (c *ResumableClient) release() {
	if c.options.Budget != nil && c.reserved > 0 {
		c.options.Budget.release(c.reserved)
	}
	c.reserved = 0
}

func (c *ResumableClient) overflow() {
	if c.options.OnOverflow != nil {
		c.options.OnOverflow(c)
		return
	}
	c.Discard()
}

// ResumableConn is one connection of a ResumableClient: the Client a
// connect request reads its messages from.
type ResumableConn struct {
	*ResumableClient

	att *attachment
}

// ensures that ResumableConn satisfies the Client interface
var _ Client = (*ResumableConn)(nil)

// Channel implements the [Client.Channel] interface method.
//
// It is closed when another connection attaches or the client is discarded.
func (c *ResumableConn) Channel() chan Message {
	return c.att.out
}

// Gen returns the connection's generation (see [ResumableClient.Detached]).
func (c *ResumableConn) Gen() uint64 {
	return c.att.gen
}

// Detach ends the connection and keeps the client's undelivered messages
// queued for a later Attach. It reports false when another connection has
// attached since, or when the client was discarded or overflowed.
func (c *ResumableConn) Detach() bool {
	return c.ResumableClient.detach(c.att.gen)
}

// QueueBudget bounds the total FIFO bytes of the detached clients sharing
// it.
type QueueBudget struct {
	mu    sync.Mutex
	max   int
	total int
}

// NewQueueBudget creates a budget of max bytes.
func NewQueueBudget(max int) *QueueBudget {
	return &QueueBudget{max: max}
}

// Total returns the bytes reserved now.
func (b *QueueBudget) Total() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.total
}

func (b *QueueBudget) reserve(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.total+n > b.max {
		return false
	}
	b.total += n
	return true
}

func (b *QueueBudget) release(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.total -= n
}

// withSeq adds `"seq":<seq>` as the first field of a JSON object payload.
// Any other payload is returned unchanged.
func withSeq(data []byte, seq uint64) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return data
	}

	rest := bytes.TrimSpace(trimmed[1:])
	field := `"seq":` + strconv.FormatUint(seq, 10)

	result := make([]byte, 0, len(trimmed)+len(field)+1)
	result = append(result, '{')
	result = append(result, field...)
	if rest[0] != '}' {
		result = append(result, ',')
	}
	result = append(result, rest...)

	return result
}
