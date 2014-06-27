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

func NewMaildir(dir string) *Maildir {
	m := &Maildir{directory: dir}
	m.create()
	return m
}

func (m *Maildir) create() {
	err := os.MkdirAll(m.directory+"/tmp", CreateMode)
	if err != nil {
		log.Println("Error creating directory tmp:", err)
		return
	}
	err = os.MkdirAll(m.directory+"/cur", CreateMode)
	if err != nil {
		log.Println("Error creating directory cur:", err)
		return
	}
	err = os.MkdirAll(m.directory+"/new", CreateMode)
	if err != nil {
		log.Println("Error creating directory new:", err)
		return
	}
}

func (m *Maildir) Deliver(f string) {
	name := path.Base(f)
	os.Rename(f, m.directory+"/new/"+name+":2,")

}

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
