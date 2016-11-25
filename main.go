package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"plugin"
)

func readLine() string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter text: ")
	text, _ := reader.ReadString('\n')
	return text
}

type PrefixLineWriter struct {
	Prefix     []byte
	Child      io.Writer
	firstWrite bool
}

func NewPrefixLineWriter(p string, c io.Writer) *PrefixLineWriter {
	return &PrefixLineWriter{[]byte(p), c, true}
}

// FIXME: returns length including prefix.
func (w *PrefixLineWriter) Write(buf []byte) (n int, err error) {
	var tn int
	if w.firstWrite {
		n, err := w.Child.Write(w.Prefix)
		w.firstWrite = false
		tn += n
		if err != nil {
			return n, err
		}
	}
	for _, b := range buf {
		n, err := w.Child.Write([]byte{b})
		tn += n
		if err != nil {
			return tn, err
		}

		if b == '\n' {
			n, err := w.Child.Write(w.Prefix)
			tn += n
			if err != nil {
				return tn, err
			}
		}
	}

	return tn, nil
}

func main() {

	commandString := readLine()

	boilerplate := `
	
package main

import "fmt"

func Cmd() error { 
` +
		commandString + `
	return nil
}
`

	tempFile, _ := ioutil.TempFile("", "repl")
	tempFile.Close()
	goTempFile, err := os.Create(tempFile.Name() + ".go")
	if err != nil {
		fmt.Println("failed to create go temp file")
		panic(err)
	}

	goTempFile.WriteString(boilerplate)
	goTempFile.Close()

	binTempFile, _ := ioutil.TempFile("", "replcmd")
	binTempFile.Close()

	sh := exec.Command("go", "build", "-buildmode=plugin", "-o", binTempFile.Name(), goTempFile.Name())
	sh.Stdout = NewPrefixLineWriter("internal: ", os.Stdout)
	sh.Stderr = NewPrefixLineWriter("internal: ", os.Stderr)
	err = sh.Run()
	if err != nil {
		fmt.Println("command failed ")
		panic(err)
	}

	p, err := plugin.Open(binTempFile.Name())
	if err != nil {
		fmt.Println("failed to open plugin " + binTempFile.Name())
		panic(err)
	}
	cmd, err := p.Lookup("Cmd")
	if err != nil {
		fmt.Println("couldn't find symbol Cmd")
		panic(err)
	}

	err = cmd.(func() error)()
	fmt.Println(err)
}
