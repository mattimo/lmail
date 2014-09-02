# lmail - lmail absolutley is lmail

lmail is a smtp server library written in go. It resembles the api provided 
by the net.http package.

## Documentation

Please refer to http://godoc.org/github.com/mattimo/lmail for documentation.

## Usage

```
import "github.com/mattimo/lmail" 
```

The easiest way to get started is to just call ListenAndServer() with a small
dummy handler.

```
type DummyHandler type{}

func (d DummyHandler) HandleMail(m *lmail.Mail) (int, error) {
	io.Copy(ioutil.Discard, mail.RawReader())
	fmt.Println(m)
}

func main {
	lmail.ListenAndServer(":2525", &DummyHandler{})
}

```

This ist just a simple example. It would read the mail and print it. Mails
are only read from the smtp connection the first time the whole message is 
read.
