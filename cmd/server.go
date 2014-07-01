package main

import (
	//	"fmt"
	"io"
	"io/ioutil"
	"lmail"
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
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	handler := &PrintHandler{}
	mux := lmail.NewDefaultMuxer()
	mux.AddRcptHandler("matti@localhost", handler)
	lmail.ListenAndServe(":2525", mux)
}
