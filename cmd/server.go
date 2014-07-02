package main

import (
	//	"fmt"
	"io"
	"io/ioutil"
	"github.com/mattimo/lmail"
	"log"
	"net/http"
	_ "net/http/pprof"
)

type PrintHandler struct {
	active bool
}

func (h *PrintHandler) HandleMail(mail *lmail.Mail) (int, error) {
	log.Printf("Handling new mail from %s", mail.From)
	for i := 0; i < 5; i++ {
		go func() {
			_, err := mail.MimeMessage()
			if err != nil {
				log.Println("Error getting mail:", err)
			}
			buf := make([]byte, 1024)
			mail.RawReader().Read(buf)
			_, err = io.Copy(ioutil.Discard, mail.RawReader())
			if err != nil {
				log.Println("Error reading message:", err)
			}
		}()
	}
	_, err := io.Copy(ioutil.Discard, mail.RawReader())
	if err != nil {
		log.Println("Error reading message:", err)
	}

	return 250, nil
}

func main() {
	log.Println("Starting lmail")
	// Start pprof for debugging purposus
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	// Get a new default Mux Handler that only routes mails to given addresses
	mux := lmail.NewDefaultMuxer()
	// get the Dummy Print Handler
	handler := &PrintHandler{}
	// Register the Print Handler with a given Address at the mux
	mux.AddRcptHandler("matti@localhost", handler)
	// Get a new Maildir instance in the current der
	maildir, err := lmail.NewMaildir("./maildir/")
	if err != nil {
		log.Fatal("Error during initialization:", err)
	}
	// Register The maildir at the muxer
	mux.AddRcptHandler("doof@localhost", maildir)
	// listen on port 2525 with the muxer
	lmail.ListenAndServe(":2525", mux)
}
