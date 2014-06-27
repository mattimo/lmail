package lmail

import (
	"bufio"
	"io"
	"net/mail"
)

// Data is only read from the connection when it is read from either the
//message Body or the Raw reader.
type Mail struct {
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
	Msg *mail.Message
	// Raw mail as recieved by the server.
	Raw io.Reader
}

func (m *Mail) PutMessage(raw io.Reader) (err error) {
	buffer := bufio.NewReader(raw)
	m.Raw = bufio.NewReader(buffer)
	m.Msg, err = mail.ReadMessage(buffer)
	return err
}
