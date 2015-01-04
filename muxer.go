package lmail

import (
	"log"
	"sync"
)

// DefaultMuxer is a simple map based muxer to.
//
// This muxer is fairly simple. you can register an address that receives its
// own handler. Each handler is called if the address is registered otherwise
// the default handler is called. The default Default handler is the
// lmail.NullHandler
type DefaultMuxer struct {
	rcptHandlers   map[string]Handler
	DefaultHandler Handler
}

// NewDefaultMuxer gets a new default muxer.
func NewDefaultMuxer() *DefaultMuxer {
	return &DefaultMuxer{
		rcptHandlers:   make(map[string]Handler),
		DefaultHandler: &NullHandler{},
	}
}

// HandleMail handles a mail and then calls the registerd Handler for the matching RCPTs.
func (m *DefaultMuxer) HandleMail(mail *Mail) (code int, err error) {
	wg := &sync.WaitGroup{}
	rChan := make(chan int, 2)
	for _, rcpt := range mail.Rcpts {
		handler := m.rcptHandlers[rcpt]
		if handler == nil {
			handler = m.DefaultHandler
		}
		wg.Add(1)
		go func(handler Handler, mail *Mail, rChan chan int, wg *sync.WaitGroup) {
			defer wg.Done()
			code, err := handler.HandleMail(mail)
			if err != nil {
				log.Println("Error in Handler:", err)
				code = 500
			}
			rChan <- code
			return
		}(handler, mail, rChan, wg)
	}

	wgChan := make(chan bool)
	go func() {
		wg.Wait()
		wgChan <- true
		return
	}()
	select {
	case code := <-rChan:
		if code != 250 {
			return code, nil
		}
	case <-wgChan:
		break
	}
	return 250, nil
}

// AddRcptHandler registers a handler for an address string.
func (m *DefaultMuxer) AddRcptHandler(match string, handler Handler) {
	m.rcptHandlers[match] = handler
}
