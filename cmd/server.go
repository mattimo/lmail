package main

import (
	"bytes"
	"fmt"
	"lmail"
	"log"
)

type PrintHandler struct {
	active bool
}

func (h *PrintHandler) HandleMail(mail *lmail.Mail) (int, error) {
	fmt.Println(mail)
	fmt.Println(mail.Msg)
	buf := &bytes.Buffer{}
	fmt.Println(buf.ReadFrom(mail.Raw))
	fmt.Println(buf.String())

	return 250, nil
}

func main() {
	log.Println("Starting lmail")
	handler := &PrintHandler{}
	lmail.ListenAndServe(":2525", handler)
}
