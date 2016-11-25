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

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter text: ")
	return reader.ReadString('\n')
}

type PrefixLineWriter struct {
	Prefix     []byte
	Child      io.Writer
	firstWrite bool
}

func NewPrefixLineWriter(p string, c io.Writer) *PrefixLineWriter {
	return &PrefixLineWriter{[]byte(p), c, true}
}

func insertIntoByteArray(buf []byte, i int, b []byte) []byte {
	return append(buf[:i+1], append(b, buf[i+1:]...)...)
}

// FIXME: returns length written including prefixes preappended to lines.
func (w *PrefixLineWriter) Write(buf []byte) (n int, err error) {

	if w.firstWrite {
		buf = insertIntoByteArray(buf,0,w.Prefix)
		w.firstWrite = false
	}

	// scan the buf for newlines, append the prefix to any we find.
	for i, b := range buf {
		if b == '\n' {
			buf = insertIntoByteArray(buf, i, w.Prefix)
		}
	}

	return w.Child.Write(buf)
}

const BoilerPlatePrologue = `
		
	package main

	import "fmt"

	func Cmd() error { 

`
const BoilerPlateEpilogue = `

		return nil
	}
`

func runCmd() {

	commandString, err := readLine()
	if err != nil {
		if err == io.EOF {
			fmt.Println("Exiting REPL")
			os.Exit(0)
		} else {
			os.Stderr.WriteString("Incomplete line read.")
			os.Exit(1)
		}
	}
	if commandString == "\n" {
		fmt.Println("Exiting REPL")
		os.Exit(0)
	}


	tempFile, _ := ioutil.TempFile("", "repl")
	tempFile.Close()
	os.Remove(tempFile.Name())

	// the file must have a .go extension to be compiled with an absolute path.
	goTempFile, err := os.Create(tempFile.Name() + ".go")
	if err != nil {
		panic("failed to create temp file: " + err.Error())
	}
	defer os.Remove(goTempFile.Name())

	code := BoilerPlatePrologue + commandString + BoilerPlateEpilogue
	goTempFile.WriteString(code)
	goTempFile.Close()

	binTempFile, err := ioutil.TempFile("", "replbin")
	if err != nil {
		panic("failed to create temp file: " + err.Error())
	}
	binTempFile.Close()
	defer os.Remove(binTempFile.Name())

	sh := exec.Command("go", "build", "-buildmode=plugin", "-o", binTempFile.Name(), goTempFile.Name())
	sh.Stdout = NewPrefixLineWriter("internal: ", os.Stdout)
	sh.Stderr = NewPrefixLineWriter("internal: ", os.Stderr)
	err = sh.Run()
	if err != nil {
		fmt.Println("build command failed: " + err.Error())
		return
	}

	p, err := plugin.Open(binTempFile.Name())
	if err != nil {
		panic("failed to open generated plugin file, '" + binTempFile.Name() + "': " + err.Error())
	}
	cmd, err := p.Lookup("Cmd")
	if err != nil {
		panic("couldn't find symbol Cmd: " + err.Error())
	}

	err = cmd.(func() error)()
	if err != nil {
		fmt.Println("An error was returned: " + err.Error())
	}
}

func main() {
	for {
		runCmd()
		fmt.Println("")
	}
}
