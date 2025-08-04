package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"log"
	"time"
)

type Server interface {
	Address() string
	IsAlive() bool
	Serve(rw http.ResponseWriter, r *http.Request)
}

type simpleServer struct {
	addr  string
	proxy *httputil.ReverseProxy
}

func newSimpleServer(addr string) *simpleServer {
	serverUrl, err := url.Parse(addr)
	handleErr(err)

	return &simpleServer{
		addr:  addr,
		proxy: httputil.NewSingleHostReverseProxy(serverUrl),
	}
}

type loadBalancer struct {
	port            string
	roundRobinCount int
	servers         []Server
}

func NewLoadBalancer(port string, servers []Server) *loadBalancer {
	return &loadBalancer{
		port:            port,
		roundRobinCount: 0,
		servers:         servers,
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Printf("error:%v\n", err)
		os.Exit(1)
	}
}
func (s *simpleServer) Address() string { return s.addr }
func (s *simpleServer) IsAlive() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(s.addr + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
func (s *simpleServer) Serve(rw http.ResponseWriter, req *http.Request) {
	s.proxy.ServeHTTP(rw, req)
}

func (lb *loadBalancer) getNextServer() Server {
	server := lb.servers[lb.roundRobinCount%len(lb.servers)]
	for !server.IsAlive() {
		lb.roundRobinCount++
		server = lb.servers[lb.roundRobinCount%len(lb.servers)]
	}
	lb.roundRobinCount++
	return server
}
func (lb *loadBalancer) serverProxy(rw http.ResponseWriter, req *http.Request) {
	targetServer := lb.getNextServer()
	fmt.Printf("forwarding request to address:%q\n", targetServer.Address())
	targetServer.Serve(rw, req)
}
func logRequestToFile(req *http.Request, targetAddr string) {
	logFile, err := os.OpenFile("requests.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("error opening log file: %v\n", err)
		return
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("Method: %s, URL: %s, Forwarded To: %s\n", req.Method, req.URL.String(), targetAddr)
}

func withLogging(lb *loadBalancer, handler func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		target := lb.getNextServer()
		logRequestToFile(req, target.Address())
		target.Serve(rw, req)
	}
}

func main() {
	servers := []Server{
		newSimpleServer("http://jsonplaceholder.typicode.com"),
		newSimpleServer("http://httpbin.org/html"),
		newSimpleServer("http://httpbin.org/json"),
		newSimpleServer("http://www.columbia.edu"),
		newSimpleServer("http://textfiles.com"),
	}
	lb := NewLoadBalancer("8085", servers)
	handleRedirect := func(rw http.ResponseWriter, req *http.Request) {
		lb.serverProxy(rw, req)
	}
	http.HandleFunc("/", withLogging(lb, handleRedirect))


	fmt.Printf("serving requests at 'localhost:%s'\n", lb.port)
	http.ListenAndServe(":"+lb.port, nil)

}
