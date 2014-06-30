package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"lmail"
	"log"
)

type PrintHandler struct {
	active bool
}

func (h *PrintHandler) HandleMail(mail *lmail.Mail) (int, error) {
	for i := 0; i < 5; i++ {
		go func() {
			fmt.Println(mail)
			mail.PrintMbuf()
			msg, err := mail.MimeMessage()
			if err != nil {
				log.Println("Error getting mail:", err)
			}
			fmt.Println("MIME MESSAGE_____________________\n", msg)
			buf := make([]byte, 1024)
			fmt.Println(mail.RawReader().Read(buf))
			fmt.Printf("RAW MESSAGE______\n%s\n", buf)
		}()
	}
	buf := make([]byte, 1024)
	fmt.Println(mail.RawReader().Read(buf))
	fmt.Printf("RAW MESSAGE______\n%s\n", buf)
	n, err := io.Copy(ioutil.Discard, mail.RawReader())
	log.Printf("Wrote %d bytes: %s", n, err)

	return 250, nil
}

func main() {
	log.Println("Starting lmail")
	handler := &PrintHandler{}
	lmail.ListenAndServe(":2525", handler)
}
