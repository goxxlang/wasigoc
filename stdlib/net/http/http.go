// Package http is HTTP/1.0 over the userspace net stack (Pipe-backed TCP).
package http

import (
	"net"
	"strconv"
	"strings"
)

type Request struct {
	Method string
	Path   string
	Host   string
	Body   string
}

type Response struct {
	Status      int
	Body        string
	ContentType string
}

type muxRoute struct {
	Path string
	Fn   func(*Request, *Response)
}

type ServeMux struct {
	routes []muxRoute
}

func NewServeMux() *ServeMux {
	return &ServeMux{}
}

func (m *ServeMux) HandleFunc(pattern string, fn func(*Request, *Response)) {
	var r muxRoute
	r.Path = pattern
	r.Fn = fn
	m.routes = append(m.routes, r)
}

func (m *ServeMux) match(path string) func(*Request, *Response) {
	if m == nil {
		return nil
	}
	for i := 0; i < len(m.routes); i++ {
		if m.routes[i].Path == path {
			return m.routes[i].Fn
		}
	}
	return nil
}

func readUntil(c *net.Conn, sep string) (string, error) {
	var out []byte
	buf := make([]byte, 1)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			out = append(out, buf[0:n]...)
			s := string(out)
			if strings.Contains(s, sep) {
				return s, nil
			}
		}
		if err != nil {
			if len(out) > 0 {
				return string(out), err
			}
			return "", err
		}
	}
}

func readN(c *net.Conn, n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	buf := make([]byte, n)
	off := 0
	for off < n {
		got, err := c.Read(buf[off:])
		if got > 0 {
			off = off + got
		}
		if err != nil {
			return string(buf[0:off]), err
		}
		if got == 0 {
			break
		}
	}
	return string(buf[0:off]), nil
}

func headerValue(raw string, key string) string {
	needle := "\n" + key + ":"
	i := strings.Index(raw, needle)
	if i < 0 {
		return ""
	}
	rest := raw[i+len(needle):]
	if len(rest) > 0 && rest[0] == 32 {
		rest = rest[1:]
	}
	e := strings.Index(rest, "\r\n")
	if e < 0 {
		return rest
	}
	return rest[0:e]
}

func parseRequest(raw string, body string) *Request {
	req := &Request{Method: "GET", Path: "/", Body: body}
	nl := strings.Index(raw, "\r\n")
	line := raw
	if nl >= 0 {
		line = raw[0:nl]
	}
	parts := strings.Split(line, " ")
	if len(parts) >= 1 {
		req.Method = parts[0]
	}
	if len(parts) >= 2 {
		req.Path = parts[1]
	}
	req.Host = headerValue(raw, "Host")
	return req
}

func writeResponse(c *net.Conn, resp *Response) {
	if resp.Status == 0 {
		resp.Status = 200
	}
	reason := "OK"
	if resp.Status == 404 {
		reason = "Not Found"
	}
	if resp.Status == 500 {
		reason = "Internal Server Error"
	}
	ct := resp.ContentType
	if ct == "" {
		ct = "text/plain"
	}
	cl := strconv.Itoa(len(resp.Body))
	st := strconv.Itoa(resp.Status)
	head := "HTTP/1.0 " + st + " " + reason + "\r\nContent-Length: " + cl + "\r\nContent-Type: " + ct + "\r\n\r\n"
	c.Write([]byte(head + resp.Body))
}

func Serve(ln *net.Listener, body string) error {
	if ln == nil {
		return nil
	}
	if body == "" {
		body = "ok"
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go serveOne(c, body)
	}
}

func serveOne(c *net.Conn, body string) {
	_, _ = readUntil(c, "\r\n\r\n")
	cl := strconv.Itoa(len(body))
	resp := "HTTP/1.0 200 OK\r\nContent-Length: " + cl + "\r\nContent-Type: text/plain\r\n\r\n" + body
	c.Write([]byte(resp))
	c.Close()
}

func ServeHandler(ln *net.Listener, mux *ServeMux) error {
	if ln == nil {
		return nil
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go serveMuxOne(c, mux)
	}
}

func serveMuxOne(c *net.Conn, mux *ServeMux) {
	raw, _ := readUntil(c, "\r\n\r\n")
	body := ""
	cl := headerValue(raw, "Content-Length")
	if cl != "" {
		n, aerr := strconv.Atoi(cl)
		if aerr == nil && n > 0 {
			body, _ = readN(c, n)
		}
	}
	req := parseRequest(raw, body)
	resp := &Response{Status: 200, ContentType: "text/plain"}
	fn := mux.match(req.Path)
	if fn == nil {
		resp.Status = 404
		resp.Body = "not found"
	} else {
		fn(req, resp)
	}
	writeResponse(c, resp)
	c.Close()
}

func parseStatus(raw string) int {
	nl := strings.Index(raw, "\r\n")
	line := raw
	if nl >= 0 {
		line = raw[0:nl]
	}
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		return 200
	}
	st, err := strconv.Atoi(parts[1])
	if err != nil {
		return 200
	}
	return st
}

func doRequest(addr string, method string, path string, body string) (int, string, error) {
	if path == "" {
		path = "/"
	}
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return 0, "", err
	}
	req := method + " " + path + " HTTP/1.0\r\nHost: " + addr + "\r\n"
	if body != "" {
		req = req + "Content-Length: " + strconv.Itoa(len(body)) + "\r\n"
	}
	req = req + "\r\n" + body
	_, werr := c.Write([]byte(req))
	if werr != nil {
		c.Close()
		return 0, "", werr
	}
	raw, rerr := readUntil(c, "\r\n\r\n")
	if rerr != nil && raw == "" {
		c.Close()
		return 0, "", rerr
	}
	rest := make([]byte, 0)
	buf := make([]byte, 256)
	for {
		n, rerr2 := c.Read(buf)
		if n > 0 {
			rest = append(rest, buf[0:n]...)
		}
		if rerr2 != nil {
			break
		}
		if n == 0 {
			break
		}
	}
	c.Close()
	return parseStatus(raw), string(rest), nil
}

func Get(addr string, path string) (int, string, error) {
	return doRequest(addr, "GET", path, "")
}

func Post(addr string, path string, body string) (int, string, error) {
	return doRequest(addr, "POST", path, body)
}
