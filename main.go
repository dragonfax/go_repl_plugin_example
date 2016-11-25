package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"plugin"
	"strings"
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
		buf = insertIntoByteArray(buf, 0, w.Prefix)
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

const CodeTemplate = `
	package main

	// imports
	%s

	func Cmd(locals map[string]interface{}) { 

		// Include locals
		// l := locals["l"].(<type>)
		%s

		// Command
		%s

		// Export new and modified local values
		// locals["l"] = l
		%s
	}
`

func applyCodeTemplate(imports []string, stmt string, vname string, addLocals bool) string {
	localIncludes := make([]string, 0, len(localVars))
	localExports := make([]string, 0, len(localVars))
	if addLocals {
		for v, val := range localVars {
			localIncludes = append(localIncludes, fmt.Sprintf("%s := locals[\"%s\"].(%T)", v, v, val))
			localExports = append(localExports, fmt.Sprintf("locals[\"%s\"] = %s", v, v))
		}
		if vname != "" {
			localExports = append(localExports, fmt.Sprintf("locals[\"%s\"] = %s", vname, vname))
		}
	}

	return fmt.Sprintf(CodeTemplate, strings.Join(imports, "\n"), strings.Join(localIncludes, "\n"), stmt, strings.Join(localExports, "\n"))
}

func getNextCommand() string {

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

	return commandString
}

func openPlugin(binTempFilename string) func(map[string]interface{}) {
	p, err := plugin.Open(binTempFilename)
	if err != nil {
		panic("Failed to open generated plugin file, '" + binTempFilename + "': " + err.Error())
	}
	cmd, err := p.Lookup("Cmd")
	if err != nil {
		panic("Couldn't find symbol Cmd: " + err.Error())
	}
	return cmd.(func(map[string]interface{}))
}

func buildCommand(binTempFilename string, goTempFilename string) bool {
	sh := exec.Command("go", "build", "-buildmode=plugin", "-o", binTempFilename, goTempFilename)
	sh.Stdout = NewPrefixLineWriter("#### ", os.Stdout)
	sh.Stderr = NewPrefixLineWriter("#### ", os.Stderr)
	err := sh.Run()
	if err != nil {
		fmt.Println("Build command failed: " + err.Error())
		return false
	}
	return true
}

func generateCode(commandString string) string {

	// Parse to find any created variable
	code := applyCodeTemplate([]string{}, commandString, "", false)
	var fset token.FileSet
	tree, err := parser.ParseFile(&fset, "console", code, parser.DeclarationErrors)
	if err != nil {
		fmt.Println("Error parsing input: " + err.Error())
		return ""
	}

	// find created variable.
	stmt := tree.Decls[0].(*ast.FuncDecl).Body.List[0]
	astmt, ok := stmt.(*ast.AssignStmt)
	vname := ""
	if ok {
		vname = astmt.Lhs[0].(*ast.Ident).Name
	}

	// Parse to find any unresolved imports
	code = applyCodeTemplate([]string{}, commandString, vname, true)
	tree, err = parser.ParseFile(&fset, "console", code, parser.DeclarationErrors)
	if err != nil {
		fmt.Println("Error parsing input: " + err.Error())
		return ""
	}

	// Add unresolved identiers, assume they are imports
	imports := make([]string, 0, 1)
	if tree.Unresolved != nil {
		for _, id := range tree.Unresolved {
			// my lazy resolver (ignore some known types)
			if id.Name == "string" || id.Name == "nil" || id.Name == "int" {
				continue
			}
			imports = append(imports, fmt.Sprintf("import \"%s\"", id.Name))
		}
	}

	// final code generation for build
	return applyCodeTemplate(imports, commandString, vname, true)
}

func saveGoCode(code string) (binFile string, goFile string) {
	tempFile, _ := ioutil.TempFile("", "repl")
	tempFile.Close()
	os.Remove(tempFile.Name())

	// the file must have a .go extension to be compiled with an absolute path.
	goTempFile, err := os.Create(tempFile.Name() + ".go")
	if err != nil {
		panic("Failed to create temp file: " + err.Error())
	}

	goTempFile.WriteString(code)
	goTempFile.Close()

	return tempFile.Name(), goTempFile.Name()
}

func runCmd() {

	commandString := getNextCommand()
	defer fmt.Println("")

	code := generateCode(commandString)
	if code == "" {
		return
	}

	binTempFilename, goTempFilename := saveGoCode(code)
	defer os.Remove(goTempFilename)
	defer os.Remove(binTempFilename)

	if !buildCommand(binTempFilename, goTempFilename) {
		return
	}

	cmd := openPlugin(binTempFilename)
	cmd(localVars)
}

var localVars map[string]interface{}

func main() {
	localVars = make(map[string]interface{})

	for {
		runCmd()
	}
}
