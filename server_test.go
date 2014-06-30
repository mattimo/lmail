package lmail

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/smtp"
	"testing"
	"time"
)

type PrintHandler struct {
	active bool
}

func (h *PrintHandler) HandleMail(mail *Mail) (int, error) {
	n, err := io.Copy(ioutil.Discard, mail.RawReader())
	if err != nil {
		return 500, fmt.Errorf("blablabla")
	}
	log.Printf("Wrote %d bytes: %s", n, err)
	return 250, nil
}

func TestConnection(t *testing.T) {
	handler := &PrintHandler{}
	go ListenAndServe(":2525", handler)
	time.Sleep(2 * time.Second)

	c, err := smtp.Dial("127.0.0.1:2525")
	if err != nil {
		t.Fatal(err)
	}
	// Set the sender and recipient first
	if err := c.Mail("sender@example.org"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rcpt("recipient@example.net"); err != nil {
		t.Fatal(err)
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(wc, "This is the email body")
	if err != nil {
		t.Fatal(err)
	}
	err = wc.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Send the QUIT command and close the connection.
	err = c.Quit()
	if err != nil {
		t.Fatal(err)
	}

}

const mailstring string = `Date: Mon, 30 Jun 2014 22:22:41 +0200
From: iniuser@ini1.ini.physik.tu-berlin.de
To: idio@localhost, doof@localhost, matti@localhost
Subject: Hello World
Message-ID: <53b1c711.1S3Fc2NHxuZqyvY8%iniuser@ini1.ini.physik.tu-berlin.de>
User-Agent: Heirloom mailx 12.5 7/5/10
MIME-Version: 1.0
Content-Type: text/plain; charset=us-ascii
Content-Transfer-Encoding: 7bit

ushdf asilasi jdha sdjasd jkhasdf
jlasgb jasdfgb adf h
asdfg asbh asjbh hsadgb hasdasdgjkasdfb adukfjghb
asdkghasgf asdgfb adhsfg
asdkgh
`

func BenchmarkConnections(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c, err := smtp.Dial("127.0.0.1:2525")
		if err != nil {
			b.Fatal(err)
		}
		// Set the sender and recipient first
		if err := c.Mail("sender@example.org"); err != nil {
			b.Fatal(err)
		}
		if err := c.Rcpt("recipient@example.net"); err != nil {
			b.Fatal(err)
		}

		// Send the email body.
		wc, err := c.Data()
		if err != nil {
			b.Fatal(err)
		}
		_, err = fmt.Fprintf(wc, mailstring)
		if err != nil {
			b.Fatal(err)
		}
		err = wc.Close()
		if err != nil {
			b.Fatal(err)
		}

		// Send the QUIT command and close the connection.
		err = c.Quit()
		if err != nil {
			b.Fatal(err)
		}
	}
	return
}
