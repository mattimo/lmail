package lmail

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"time"
)

const CreateMode os.FileMode = 0700

// Maildir is a mail Handler That saves into a maildir. Maildir is an easy way
// to store mails. For reference how to retrieve mail from a maildir refer to:
// 	http://cr.yp.to/proto/maildir.html
// This maildir implementation is supposed to read incoming mails from the
// receiving Socket into a new File in the maildirs /tmp directory and then
// move it to /new.
type Maildir struct {
	// specify where maildir structure starts
	directory string
}

func createUniqueName() (string, error) {
	unixNano := time.Now().UnixNano()
	pid := os.Getpid()
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%s", unixNano, pid, hostname), nil
}

// NewMaildir creates a new maildir at the given location. If the underlying
// directory structure does not exist, it is created. It returns a usable
// maildir struct and any errors the occure during initialisation.
func NewMaildir(dir string) (*Maildir, error) {
	m := &Maildir{directory: dir}
	err := m.create()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Maildir) create() error {
	err := os.MkdirAll(m.directory+"/tmp", CreateMode)
	if err != nil {
		err = fmt.Errorf("error creating directory tmp: %s", err)
		return err
	}
	err = os.MkdirAll(m.directory+"/cur", CreateMode)
	if err != nil {
		err = fmt.Errorf("error creating directory cur: %s", err)
		return err
	}
	err = os.MkdirAll(m.directory+"/new", CreateMode)
	if err != nil {
		err = fmt.Errorf("error creating directory new: %s", err)
		return err
	}
	return nil
}

// Deliver an email. If the mail was regarded as ok it shall be deliverd.
// This method only takes a string of the maildir file and moves it from
// tmp to new
func (m *Maildir) Deliver(f string) error {
	name := path.Base(f)
	return os.Rename(f, m.directory+"/new/"+name+":2,")

}

// StoreTmp stores a mail in the maildir. takes a reader and returns how many
// bytes where read and an error
func (m *Maildir) StoreTmp(reader io.Reader) (int64, *os.File, error) {
	unique, err := createUniqueName()
	if err != nil {
		return 0, nil, err
	}
	filename := m.directory + "/tmp/" + unique
	file, err := os.Create(filename)
	if err != nil {
		return 0, nil, err
	}
	err = file.Chmod(CreateMode)
	if err != nil {
		return 0, nil, err
	}
	n, err := io.Copy(file, reader)
	if err != nil {
		return 0, nil, err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return 0, nil, err
	}
	log.Printf("Maildir: saved %d bytes into %s\n", n, filename)
	return n, file, nil
}

// HandleMail is a simple handler, for mails that shall be stored.
func (m *Maildir) HandleMail(mail *Mail) (code int, err error) {
	_, f, err := m.StoreTmp(mail.RawReader())
	if err != nil {
		return 500, err
	}
	m.Deliver(f.Name())
	return 250, nil
}
