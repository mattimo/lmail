package lmail

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/mail"
	"sync"
)

// Takes care of the mail, Buffers it in memory.
type mailBuffer struct {
	buf *bytes.Buffer // Raw Buffer where mail is stored temporarily, NEVER read from this
	// Fancy io.TeeRader, connected to the origin buffer and to buf.
	// Read from this and the read content is written to buf transparently.
	tee io.Reader
	pos int64 // reader position
	rm  *sync.Mutex
}

func newMailBuffer(origin io.Reader) *mailBuffer {
	buf := &bytes.Buffer{}
	b := &mailBuffer{
		buf: buf,
		tee: io.TeeReader(origin, buf),
		rm:  &sync.Mutex{},
	}
	return b
}

// Clone a Buffer
func (b *mailBuffer) clone() *mailBuffer {
	return &mailBuffer{
		tee: b.tee,
		buf: b.buf,
		pos: 0,
		rm:  b.rm,
	}
}

// Opportunistic Reader, Reads from origin buffer through the tee reader iff
// the position has not been read before.
// TODO: get rid of that silly lock.
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

// Mail type represents mail data and is passed between handlers.
//
// Data is only read from the connection when it is read from either the
// message Body or the Raw reader.
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

// PutMessage puts a raw mail to the buffer. Takes an io.Reader as an argument.
func (m *Mail) PutMessage(raw io.Reader) {
	m.mailBuf = newMailBuffer(raw)
}

// RawReader returns a raw Reader for the Message. The returned reader can be 
// read from several goroutines simultaniously.
func (m *Mail) RawReader() io.Reader {
	return m.mailBuf.clone()
}

// MimeMessage returns the mime header from the message. If the header could
// not be read it returns an error.
func (m *Mail) MimeMessage() (msg *mail.Message, err error) {
	if m.msg == nil {
		mailReader := m.RawReader()
		msg, err = mail.ReadMessage(mailReader)
		m.msg = msg
		return
	}
	return m.msg, nil
}

// NullHandler is a simple handler that discards the mail. This Handler reads
// from the raw buffer until io.EOF and discards the readers contents.
type NullHandler struct{}

// HandleMail callback that just discards the mail.
func (d *NullHandler) HandleMail(m *Mail) (code int, err error) {
	n, err := io.Copy(ioutil.Discard, m.RawReader())
	if err != nil {
		return 500, err
	}
	log.Printf("NullHandler: Copied %d bytes to /dev/null", n)
	return 250, nil
}
