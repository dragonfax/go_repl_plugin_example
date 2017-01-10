
### What is it?

A simple example of using the new Go 1.8 plugin system to implement a Go REPL.

### What it Does

With this technique, there is only ever one Go process. The code you type is compiled live, and then loaded into the existing go process, as a Golang Plugin (like a shared library).

Then its executed within the memoryspace of that single go process. No child processes are created, no RPC is done, no code is interpreted. 

Local variables are captured and available each new command you type. The command-plugin generation step includes them via some simple boilerplate templating. And they're current values are passed into the plugin by reference.

### How To

1. `go run main.go`
2. Then type a line of code.

### See it Work

```
$ go run main.go
Enter text: fmt.Println("test")
test

Enter text: l := 1

Enter text: j := 2

Enter text: s := l + j

Enter text: fmt.Printf("%d",s)
3
Enter text: 
Exiting REPL
$
```

### Why

An experiment and an excuse to try out the plugin system new in v1.8.

But REPLs have their purpose. You can't beat the convenience of REPL when doing adhoc analysis and data munging.

### Whats Next

This is a very naive implementation. There is a lot to be done before this is a genuinely usable tool.

* Remove some of the shell and go layers when building the plugin
* Multi-line input (blocks)
* Take a wider range of expressions and statements. (funcs and constants, import statements and such)
* better auto imports
* History tracking
* much much more
