package lmail

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/mail"
	"sync"
)

type mailBuffer struct {
	buf    *bytes.Buffer // Raw Buffer where mail is stored temporarily, NEVER read from this
	origin io.Reader     // buffer origin
	tee    io.Reader
	pos    int64 // reader position
	rm     *sync.Mutex
}

func newMailBuffer(origin io.Reader) *mailBuffer {
	buf := &bytes.Buffer{}
	b := &mailBuffer{
		buf:    buf,
		origin: origin,
		tee:    io.TeeReader(origin, buf),
		rm:     &sync.Mutex{},
	}
	return b
}

func (b *mailBuffer) Clone() *mailBuffer {
	return &mailBuffer{
		tee: b.tee,
		buf: b.buf,
		pos: 0,
		rm:  b.rm,
	}
}

// Opportunistic Reader, Reads from origin buffer iff the position has not
// been read before.
func (b *mailBuffer) Read(p []byte) (n int, err error) {
	b.rm.Lock()
	n, err = b.tee.Read(p)
	if err != nil {
		if err != io.EOF {
			b.rm.Unlock()
			return
		}
	}
	if n > 0 && err != io.EOF {
		b.pos = b.pos + int64(n)
		b.rm.Unlock()
		return
	}
	if int64(len(b.buf.Bytes())) < b.pos {
		panic("Well this shouldn't happen")
	}
	data := b.buf.Bytes()
	b.rm.Unlock()
	n, err = bytes.NewReader(data).ReadAt(p, b.pos)
	b.pos = b.pos + int64(n)
	return
}

// Data is only read from the connection when it is read from either the
//message Body or the Raw reader.
type Mail struct {
	mailBuf *mailBuffer
	// Server Client name. We use the reverse Lookup of the client
	//connection
	Client string
	// Client connection name as advertised by the client itself
	ClientName string
	// Mail sender as advertised by client
	From string
	// Slice of reciepients as registered by the client
	Rcpts []string
	// Parsed Message
	msg *mail.Message
}

func (m *Mail) PutMessage(raw io.Reader) (err error) {
	m.mailBuf = newMailBuffer(raw)
	return err
}

func (m *Mail) RawReader() io.Reader {
	return m.mailBuf.Clone()
}

func (m *Mail) MimeMessage() (msg *mail.Message, err error) {
	if m.msg == nil {
		mailReader := m.RawReader()
		msg, err = mail.ReadMessage(mailReader)
		m.msg = msg
		return
	}
	return m.msg, nil
}

type DefaultHandler struct{}

func (d *DefaultHandler) HandleMail(m *Mail) (code int, err error) {
	n, err := io.Copy(ioutil.Discard, m.RawReader())
	if err != nil {
		return 500, err
	}
	log.Printf("DefaultHandler: Copied %d bytes to /dev/null", n)
	return 250, nil
}
