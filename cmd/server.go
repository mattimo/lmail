package main

import (
	//	"fmt"
	"io"
	"io/ioutil"
	"lmail"
	"log"
)

type PrintHandler struct {
	active bool
}

func (h *PrintHandler) HandleMail(mail *lmail.Mail) (int, error) {
	log.Printf("Handling new mail from %s", mail.Client)
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
	handler := &PrintHandler{}
	lmail.ListenAndServe(":2525", handler)
}
