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

		// import locals
		%s

		// Command
		%s

		// export new and modified local values
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

	// Parse to find any created variable
	code := applyCodeTemplate([]string{}, commandString, "", false)
	var fset token.FileSet
	tree, err := parser.ParseFile(&fset, "console", code, parser.DeclarationErrors)
	if err != nil {
		fmt.Println("Error parsing input: " + err.Error())
		return
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
		return
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
	code = applyCodeTemplate(imports, commandString, vname, true)

	tempFile, _ := ioutil.TempFile("", "repl")
	tempFile.Close()
	os.Remove(tempFile.Name())

	// the file must have a .go extension to be compiled with an absolute path.
	goTempFile, err := os.Create(tempFile.Name() + ".go")
	if err != nil {
		panic("Failed to create temp file: " + err.Error())
	}
	defer os.Remove(goTempFile.Name())

	goTempFile.WriteString(code)
	goTempFile.Close()

	binTempFile, err := ioutil.TempFile("", "replbin")
	if err != nil {
		panic("Failed to create temp file: " + err.Error())
	}
	binTempFile.Close()
	defer os.Remove(binTempFile.Name())

	sh := exec.Command("go", "build", "-buildmode=plugin", "-o", binTempFile.Name(), goTempFile.Name())
	sh.Stdout = NewPrefixLineWriter("#### ", os.Stdout)
	sh.Stderr = NewPrefixLineWriter("#### ", os.Stderr)
	err = sh.Run()
	if err != nil {
		fmt.Println("Build command failed: " + err.Error())
		return
	}

	p, err := plugin.Open(binTempFile.Name())
	if err != nil {
		panic("Failed to open generated plugin file, '" + binTempFile.Name() + "': " + err.Error())
	}
	cmd, err := p.Lookup("Cmd")
	if err != nil {
		panic("Couldn't find symbol Cmd: " + err.Error())
	}

	cmd.(func(map[string]interface{}))(localVars)
}

var localVars map[string]interface{}

func main() {
	localVars = make(map[string]interface{})

	for {
		runCmd()
		fmt.Println("")
	}
}
