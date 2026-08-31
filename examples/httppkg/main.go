package main

import (
	"fmt"
	"net"
	"net/http"
)

func handleRoot(req *http.Request, resp *http.Response) {
	resp.Body = "ok"
}

func handleEcho(req *http.Request, resp *http.Response) {
	resp.Body = req.Method + " " + req.Path + " " + req.Body
}

func main() {
	ln, err := net.Listen("tcp", ":8081")
	fmt.Println(err == nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/echo", handleEcho)
	go http.ServeHandler(ln, mux)
	st, body, _ := http.Get(":8081", "/")
	fmt.Println(st)
	fmt.Println(body)
	st2, body2, _ := http.Post(":8081", "/echo", "hi")
	fmt.Println(st2)
	fmt.Println(body2)
	st3, body3, _ := http.Get(":8081", "/nope")
	fmt.Println(st3)
	fmt.Println(body3)
	ln.Close()
}
