package lmail

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// servers name as stated in the initial connect 250
const serverName string = "ini1.ini.physik.tu-berlin.de"

// preliminary location to store extension list supported by the server
var extensions []string = []string{"8BITMIME", "SIZE"}

// The server timour is set to 5 minuted as proposed in rfc5321 4.5.3.2.7.
const timeoutTime time.Duration = 5 * time.Minute

// Processing time out is set to 8 hours because it seems reasonable
const processingTimeout time.Duration = 8 * time.Hour

// Maildir instance, shall contain folder name
var maildir *Maildir

// Mail Handlers Process Mails,
type Handler interface {
	// Handles Mails, gets passed a mail struct as an argument, and should return
	// an smtp error, and an error object for all other errors.
	// If the smtp error code is 0 or err is not nil it is ignored.
	// if err is not nil, the server will respond with the appropriate
	// error code or ignore the handler
	HandleMail(*Mail) (int, error)
}

type session struct {
	// raw network connection
	conn net.Conn
	// Textproto context
	text *textproto.Conn
	// check if session is still active
	active bool
	// check if EHLO/HELO ran yet
	pastHello bool
	// server timeout
	timeout *time.Timer
	// connection time out
	timedout bool

	// The mail the is beeing received.
	mail *Mail

	// Delivery Function
	handle func(*Mail) (int, error)
	// Verify Function
	Verify func(io.ReadWriter) (bool, error)
}

func NewSession(conn net.Conn) *session {
	s := &session{
		conn: conn,
		text: textproto.NewConn(conn),
		mail: &Mail{},
	}
	s.reset()

	s.timeout = time.AfterFunc(timeoutTime, s.Close)
	return s

}

func (s *session) reset() {
	s.active = true
	s.pastHello = false
	s.timedout = false
}

func (s *session) ResetTimeout() {
	s.timeout.Reset(timeoutTime)
}

func (s *session) Close() {
	s.timedout = true
	s.timeout.Stop()
	err := s.text.Close()
	if err != nil {
		log.Println("Erro failed to close Session:", err)
	}
}

func getTouple(field string) (string, string, error) {
	kv := strings.Split(field, ":")
	if len(kv) != 2 {
		return "", "", fmt.Errorf("No fields found")
	}
	return kv[0], kv[1], nil
}

func getAddress(value string) string {
	value = strings.TrimLeft(value, "<")
	return strings.TrimRight(value, ">")
}

func (s *session) handleClose() {
	s.active = false
	s.Cmd(221, "OK")
	s.Close()
}

func (s *session) handleHelo(args []string) {
	if len(args) < 2 {
		s.Cmd(553, "mailbox syntax incorrect")
		return
	}
	client := args[1]
	// TODO: validate URL
	s.Cmd(250, "Hello %s, use EHLO, motherfucker.", client)
	s.pastHello = true
}

func (s *session) replyExtensions(client string) {
	rAddrHostPort := s.conn.RemoteAddr().String()
	rAddr, _, err := net.SplitHostPort(rAddrHostPort)
	names, err := net.LookupAddr(rAddr)
	if err != nil {
		log.Println("Error During reverse Lookup:", err)
		s.Cmd(451, "That didn't work")
		return
	}
	var name string
	if len(names) == 0 {
		name = client
	} else {
		name = names[0]
	}
	s.mail.Client = name
	s.Ecmd(250, "%s, Hello %s [%s]", serverName, name, rAddr)
	for _, extension := range extensions[:1] {
		s.Ecmd(250, extension)
	}
	s.Cmd(250, extensions[len(extensions)-1])
}

func (s *session) handleEhlo(args []string) {
	if len(args) < 2 {
		s.Cmd(553, "mailbox syntax incorrect")
		return
	}
	client := args[1]
	// TODO: validate URL
	s.replyExtensions(client)
	s.pastHello = true
}

func (s *session) handleMail(args []string) {
	// check from field
	if len(args) < 2 {
		s.Cmd(553, "mailbox syntax incorrect")
		return
	}
	k, v, err := getTouple(args[1])
	defer func() {
		if err != nil {
			s.Cmd(553, "mailbox syntax incorrect")
		} else {
			s.Cmd(250, "OK")
		}
	}()

	if err != nil {
		log.Println("MAIL: Error parsing touple:", err)
		return
	}
	if k != "FROM" {
		err = fmt.Errorf("wrong key name")
		log.Println("MAIL: Error no \"FROM\" Field:", err)
		return
	}
	from, err := mail.ParseAddress(v)
	if err != nil {
		log.Println("MAIL: Error Parsing Address:", err)
		return
	}
	s.mail.From = from.Address
	return

}

func (s *session) handleRcpt(args []string) {
	// check recepient
	if len(args) < 2 {
		s.Cmd(553, "mailbox syntax incorrect")
		return
	}
	k, v, err := getTouple(args[1])
	defer func() {
		if err != nil {
			s.Cmd(553, "mailbox syntax incorrect")
		} else {
			s.Cmd(250, "OK")
		}
	}()
	if err != nil {
		log.Println("RCPT: Error parsing touple:", err)
		return
	}
	if k != "TO" {
		err = fmt.Errorf("Wrong key name")
		log.Println("RCPT: Error no \"TO\" Field:", err)
		return
	}
	rcpt, err := mail.ParseAddress(v)
	if err != nil {
		log.Println("RCPT: Error Parsing Address:", err)
		return
	}
	s.mail.Rcpts = append(s.mail.Rcpts, rcpt.Address)
	return

}

// compare list of RCPTs with recepients found in MIME header
// return true if all recepients could be found in either list, false if not.
// Error contains how many recepients could not be matched.
func (s *session) rcptMimeMatch(header mail.Header) (bool, error) {
	// unique recepients, bool is false if key was not found in MIME header
	uRecepients := make(map[string]bool)
	// list of destination field names
	fields := []string{"To", "Cc", "Bcc"}
	// mismatch counter
	count := 0
	for _, rcpt := range s.mail.Rcpts {
		uRecepients[rcpt] = false
		for _, key := range fields {
			addresses, err := header.AddressList(key)
			if err != nil {
				continue
			}
			for _, addr := range addresses {
				// check if exists and write appropriate value into fields
				if _, ok := uRecepients[addr.Address]; ok {
					uRecepients[addr.Address] = true
				} else {
					uRecepients[addr.Address] = false
				}
			}
		}
	}
	for _, uRcpt := range uRecepients {
		if !uRcpt {
			count++
		}
	}
	if count != 0 {
		return false, fmt.Errorf("Recepient mismatch for %d RCPTs", count)
	}
	return true, nil
}

func (s *session) handleData(args []string) {
	if s.mail.From == "" {
		s.Cmd(503, "FROM sequence must come before DATA")
		return
	}
	if len(s.mail.Rcpts) == 0 {
		s.Cmd(503, "RCPT sequnce must come before DATA")
		return
	}
	s.Cmd(354, "Ready to receive mails end with single . line")

	var readError error
	defer func() {
		if readError != nil {
			log.Println("Error during data receive")
			s.Cmd(550, readError.Error())
		} else {
			s.Cmd(250, "OK")
		}
	}()
	dataReader := s.text.DotReader()
	err := s.mail.PutMessage(dataReader)
	if err != nil {
		log.Println("Error reading from con:", err)
		readError = fmt.Errorf("Error reading data")
		return
	}
	//TODO handle smtp error code
	_, err = s.handle(s.mail)
	if err != nil {
		log.Println("Error reading from con:", err)
		readError = fmt.Errorf("Error reading data")
	}
	if false {
		_, file, err := maildir.StoreTmp(dataReader)
		if err != nil {
			log.Println("Error reading from con:", err)
			readError = fmt.Errorf("Error reading data")
			return
		}
		msg, err := mail.ReadMessage(file)
		if err != nil {
			log.Println("Error reading message", err)
			readError = fmt.Errorf("Error reading data")
			return
		}
		ok, err := s.rcptMimeMatch(msg.Header)
		if !ok {
			log.Println("Error matching MIME:", err)
			readError = fmt.Errorf("Error reading data")
			return
		}
	}
}

func (s *session) handleRset(args []string) {
	s.reset()
	s.Cmd(250, "OK")
}

func (s *session) handleNoop(args []string) {
	s.Cmd(250, "OK")
}

func (s *session) handleVrfy(args []string) {
	// TODO: implement real vrfy, just pretend that we prohibit this
	// behaviour by policy
	s.Cmd(252, "Administrative prohibition")
}

// send Normal Command with number and command text
func (s *session) Cmd(code int, message string, args ...interface{}) error {
	s.active = true
	pmsg := fmt.Sprintf("%d %s", code, message)
	return s.text.PrintfLine(pmsg, args...)
}

// send multiline command string
func (s *session) Ecmd(code int, message string, args ...interface{}) error {
	s.active = true
	pmsg := fmt.Sprintf("%d-%s", code, message)
	return s.text.PrintfLine(pmsg, args...)
}

// TODO: certainly not the correct name
func (s *session) serverHello(server string) {
	s.Cmd(220, "%s ESMTP lmail", server)
}

func (srv *Server) handleConnection(conn net.Conn) {
	s := NewSession(conn)
	s.serverHello(srv.Name)
	s.handle = srv.Handler.HandleMail
	for s.active {
		// reset timeout to prevent clients from dangling around
		s.ResetTimeout()
		line, err := s.text.ReadLine()
		if err != nil {
			if !s.timedout {
				log.Println("Error reading line:", err)
			}
			return
		}
		// Stop the timout for the time beeing, we are in the middle of something
		s.timeout.Reset(processingTimeout)
		args := strings.Fields(line)
		if len(args) == 0 {
			continue
		}
		// handle stateless commands
		switch args[0] {
		case "RSET":
			s.handleRset(args)
			continue
		case "NOOP":
			s.handleNoop(args)
			continue
		case "QUIT":
			s.handleClose()
			return
		case "VRFY":
			s.handleVrfy(args)
			continue
		}
		if s.pastHello {
			switch args[0] {
			case "MAIL":
				s.handleMail(args)
				continue
			case "RCPT":
				s.handleRcpt(args)
				continue
			case "DATA":
				s.handleData(args)
				continue
			default:
				s.Cmd(500, "Syntax error, command unrecognized")
				continue
			}
		} else {
			switch args[0] {
			case "EHLO":
				s.handleEhlo(args)
				continue
			case "HELO":
				s.handleHelo(args)
				continue
			default:
				s.Cmd(503, "Hello must come before anything else")
			}
		}
	}
}

type Server struct {
	Addr    string  //TCP address to listen on, ":smtp" if empty
	Handler Handler // handler to invoke, lmail.DefaultServeMux if nil
	Name    string  // Server name, hostname if emtpy

	// Error Logger, if nil logs are deferred.
	ErrorLog *log.Logger
}

func (srv *Server) logf(format string, args ...interface{}) {
	if srv.ErrorLog != nil {
		srv.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":smtp"
	}
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("Could not Listen")
	}
	return srv.Serve(listen)
}

func (srv *Server) Serve(l net.Listener) error {
	defer l.Close()
	if srv.Name == "" {
		name, err := os.Hostname()
		if err != nil {
			return err
		}
		srv.Name = name
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println("Error During Connect:", err)
			continue
		}
		go srv.handleConnection(conn)

	}
}

func ListenAndServe(addr string, handler Handler) error {
	srv := &Server{Addr: addr, Handler: handler}
	return srv.ListenAndServe()
}
