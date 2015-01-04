package lmail

import (
	"crypto/tls"
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

// preliminary location to store extension list supported by the server
var extensions []string = []string{"8BITMIME", "SIZE", "STARTTLS"}

// The server timour is set to 5 minuted as proposed in rfc5321 4.5.3.2.7.
const timeoutTime time.Duration = 5 * time.Minute

// Processing time out is set to 8 hours because it seems reasonable
const processingTimeout time.Duration = 8 * time.Hour

// Maildir instance, shall contain folder name
var maildir *Maildir

// Handler interface
type Handler interface {
	// Handles Mails, gets passed a mail struct as an argument, and should return
	// an smtp error, and an error object for all other errors.
	// If the smtp error code is 0 or err is not nil it is ignored.
	// if err is not nil, the server will respond with the appropriate
	// error code or ignore the handler
	HandleMail(*Mail) (int, error)
}

type session struct {
	conn      net.Conn        // raw network connection
	text      *textproto.Conn // Textproto context
	active    bool            // check if session is still active
	pastHello bool            // check if EHLO/HELO ran yet
	starttls  bool            // true if tls session
	timeout   *time.Timer     // server timeout
	timedout  bool            // connection time out
	mail      *Mail           // The mail the is beeing received.
	server    *Server         // The server whom initiated the session

	// Delivery Function
	handle func(*Mail) (int, error)
	// Verify Function
	Verify func(io.ReadWriter) (bool, error)
}

func NewSession(conn net.Conn, server *Server) *session {
	s := &session{
		conn:   conn,
		text:   textproto.NewConn(conn),
		mail:   &Mail{},
		server: server,
	}
	s.reset()
	s.timeout = time.AfterFunc(timeoutTime, func() {
		s.Close()
	})
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

func (s *session) Close() error {
	s.timedout = true
	s.timeout.Stop()
	err := s.text.Close()
	if err != nil {
		return fmt.Errorf("failed to close Session: %s", err)
	}
	return nil
}

func getTouple(field string) (string, string, error) {
	kv := strings.Split(field, ":")
	if len(kv) != 2 {
		return "", "", fmt.Errorf("no fields found")
	}
	return kv[0], kv[1], nil
}

func getAddress(value string) string {
	value = strings.TrimLeft(value, "<")
	return strings.TrimRight(value, ">")
}

func (s *session) handleClose() {
	s.active = false
	s.Cmd(CodeClosing, "OK")
	s.Close()
}

func (s *session) handleHelo(args []string) {
	if len(args) < 2 {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	client := args[1]
	// TODO: validate URL
	s.Cmd(CodeOk, "Hello %s, use EHLO, motherfucker.", client)
	s.pastHello = true
}

func (s *session) replyExtensions(client string) error {
	rAddrHostPort := s.conn.RemoteAddr().String()
	rAddr, _, err := net.SplitHostPort(rAddrHostPort)
	names, err := net.LookupAddr(rAddr)
	// TODO: this isnt' very smart but it does the job, we just have to
	// disconnect if the address can't be lookd up.
	if err != nil {
		s.Cmd(CodeAborted, "That didn't work")
		return fmt.Errorf("reverse lookup failed: %s", err)
	}
	var name string
	if len(names) == 0 {
		name = client
	} else {
		name = names[0]
	}
	s.mail.Client = name
	s.Ecmd(CodeOk, "%s, Hello %s [%s]", s.server.Name, name, rAddr)
	for _, extension := range extensions[:1] {
		s.Ecmd(CodeOk, extension)
	}
	s.Cmd(CodeOk, extensions[len(extensions)-1])
	return nil
}

func (s *session) handleEhlo(args []string) error {
	if len(args) < 2 {
		s.ErrCmd(CodeSyntaxError)
		return nil
	}
	client := args[1]
	// TODO: validate URL
	err := s.replyExtensions(client)
	if err != nil {
		return err
	}
	s.pastHello = true
	return nil
}

func (s *session) handleMail(args []string) {
	// check from field
	if len(args) < 2 {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	k, v, err := getTouple(args[1])
	if err != nil {
		s.ErrCmd(CodeSyntaxError)
		return
	}

	if k != "FROM" {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	from, err := mail.ParseAddress(v)
	if err != nil {
		s.ErrCmd(CodeMailboxNameNotAllowed)
		return
	}
	s.mail.From = from.Address
	s.Cmd(CodeOk, "OK")
	return

}

func (s *session) handleRcpt(args []string) {
	// check recepient
	if len(args) < 2 {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	k, v, err := getTouple(args[1])
	if err != nil {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	if k != "TO" {
		s.ErrCmd(CodeSyntaxError)
		return
	}
	rcpt, err := mail.ParseAddress(v)
	if err != nil {
		s.ErrCmd(CodeMailboxNameNotAllowed)
		return
	}
	s.mail.Rcpts = append(s.mail.Rcpts, rcpt.Address)
	s.Cmd(CodeOk, "OK")
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
		return false, fmt.Errorf("recepient mismatch for %d RCPTs", count)
	}
	return true, nil
}

func (s *session) handleData(args []string) error {
	if s.mail.From == "" {
		s.Cmd(CodeBadSequence, "FROM sequence must come before DATA")
		return nil
	}
	if len(s.mail.Rcpts) == 0 {
		s.Cmd(CodeBadSequence, "RCPT sequnce must come before DATA")
		return nil
	}
	s.Cmd(CodeStartMailInput, "Ready to receive mails end with single . line")

	dataReader := s.text.DotReader()
	s.mail.PutMessage(dataReader)

	code, err := s.handle(s.mail)
	if err != nil {
		s.ErrCmd(CodeNotTaken)
		return fmt.Errorf("failed to handle mail: %s", err)
	}
	if code != 0 && code != CodeOk {
		s.Cmd(code, "Error during processing")
		return nil
	}
	s.Cmd(CodeOk, "OK")
	return nil
}

func (s *session) handleRset(args []string) {
	s.reset()
	s.Cmd(CodeOk, "OK")
}

func (s *session) handleNoop(args []string) {
	s.Cmd(CodeOk, "OK")
}

func (s *session) handleVrfy(args []string) {
	// TODO: implement real vrfy, just pretend that we prohibit this
	// behaviour by policy
	s.Cmd(CodeUserNoVerify, "Administrative prohibition")
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

func (s *session) ErrCmd(code int) error {
	msg, ok := SmtpErrors[code]
	if !ok {
		return s.Cmd(code, "")
	}
	return s.Cmd(code, msg)
}

// TODO: certainly not the correct name
func (s *session) serverHello(server string) {
	s.Cmd(CodeReady, "%s ESMTP lmail", server)
}

func (srv *Server) handleConnection(conn net.Conn, starttls bool) {
	t := time.Now()
	s := NewSession(conn, srv)
	s.starttls = starttls
	s.handle = srv.Handler.HandleMail

	if !s.starttls {
		s.serverHello(srv.Name)
	} else {
		srv.logf("Starttls session with %s", conn.RemoteAddr())
	}
	for s.active {
		// reset timeout to prevent clients from dangling around
		s.ResetTimeout()
		line, err := s.text.ReadLine()
		if err != nil {
			if !s.timedout {
				srv.logf("Error reading line: %s", err)
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
			srv.logf("Session Closed %s after start", time.Since(t).String())
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
				err = s.handleData(args)
				if err != nil {
					srv.logf("Error handleData: %s", err)
				}
				continue
			case "STARTTLS":
				if !s.starttls {
					s.Cmd(CodeReady, "Go ahead")
					err = srv.startTls(conn)
					if err != nil {
						srv.logf("Error startTls: %s", err)
						s.Cmd(CodeTlsNotAvaiable, "Could not start TLS")
						continue
					} else {
						return
					}
				}
			default:
				s.ErrCmd(CodeNotRecognized)
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
				s.Cmd(CodeBadSequence, "Hello must come before anything else")
			}
		}
	}
}

func (srv *Server) startTls(conn net.Conn) error {
	if srv.TLSConfig == nil {
		return fmt.Errorf("TLS Config was not set")
	}
	tlsConn := tls.Server(conn, srv.TLSConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return err
	}
	srv.handleConnection(tlsConn, true)
	return nil

}

// Server type that implements a simple smtp server
type Server struct {
	Addr    string  //TCP address to listen on, ":smtp" if empty
	Handler Handler // handler to invoke, lmail.DefaultServeMux if nil
	Name    string  // Server name, hostname if emtpy

	// TLS config to use when a starttls session is initiated by the
	// client if nil, starttls will fail.
	TLSConfig *tls.Config

	// Error Logger, if nil logs are sent to os.Stderr.
	ErrorLog *log.Logger
}

func (srv *Server) logf(format string, args ...interface{}) {
	if srv.ErrorLog != nil {
		srv.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// ListenAndServe listens on the TCP network address srv.Addr and then
// calls Serve to handle requests on incoming connections.  If
// srv.Addr is blank, ":smtp" is used.
func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":smtp"
	}
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		srv.logf("Could not Listen: %s", err)
		return err
	}
	return srv.Serve(listen)
}

// ListenAndServeTLS listens on the TCP network address srv.Addr and
// then calls Serve With startls enabled to handle requests on
// incoming connections.
//
// Filenames containing a certificate and matching private key for
// the server must be provided. If the certificate is signed by a
// certificate authority, the certFile should be the concatenation
// of the server's certificate followed by the CA's certificate.
//
// If srv.Addr is blank, ":smtp" is used. if there is an error parsing
// the certificates, we return an errror
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":smtp"
	}
	config := &tls.Config{}
	if srv.TLSConfig != nil {
		*config = *srv.TLSConfig
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	srv.TLSConfig = config
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		srv.logf("Could not Listen: %s", err)
		return err
	}
	return srv.Serve(listen)
}

// Serve accepts incoming connections on the Listener l, creating a new
// connection handler goroutine for each and which then calls a handler.
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
			srv.logf("Error During Connect: %s", err)
			continue
		}
		go srv.handleConnection(conn, false)

	}
}

// ListenAndServe Listens on a TCP network address and then calls Serve with
// the given handler.
//
// A trivial example server is:
//
// 	import (
//		"fmt"
//		"io"
//		"lmail"
//	)
//
//	type MyHandler struct {}
//
//	func (h *MyHandler) HandleMail(m *Mail) (code int, err error) {
//		var buf []bytes
//		reader := m.RawReader()
//		for n, err := reader.Read(buf); n > 0 {
//			if err != nil && err != io.EOF {
//				return 501, err
//			}
//			fmt.Printnf("%s", buf)
//		}
//
//	}
//
//	func main() {
//		handler := &MyHandler{}
//		fmt.Println(lmail.ListenAndServe(":2525", handler))
//	}
func ListenAndServe(addr string, handler Handler) error {
	srv := &Server{Addr: addr, Handler: handler}
	return srv.ListenAndServe()
}

// ListenAndServeTLS is the STARTTLS enabled clone of ListenAndServe Listens
// on a TCP network address and then calls Serve with the given handler.
// Additionally, files containing a certificate and
// matching private key for the server must be provided. If the certificate
// is signed by a certificate authority, the certFile should be the concatenation
// of the server's certificate followed by the CA's certificate.
//
// A trivial example server is:
//
// 	import (
//		"fmt"
//		"io"
//		"lmail"
//	)
//
//	type MyHandler struct {}
//
//	func (h *MyHandler) HandleMail(m *Mail) (code int, err error) {
//		var buf []bytes
//		reader := m.RawReader()
//		for n, err := reader.Read(buf); n > 0 {
//			if err != nil && err != io.EOF {
//				return 501, err
//			}
//			fmt.Printnf("%s", buf)
//		}
//
//	}
//
//	func main() {
//		handler := &MyHandler{}
//		err := lmail.ListenAndServeTLS(":2525", "server.pen", server.key, handler)
//		if err != nil {
//			fmt.Println("Server Failure:", err)
//		}
//	}
//
// One can use generate_cert.go in crypto/tls to generate cert.pem and key.pem.
func ListenAndServeTLS(addr, certFile, keyFile string, handler Handler) error {
	srv := &Server{Addr: addr, Handler: handler}
	return srv.ListenAndServeTLS(certFile, keyFile)
}
