package main

import (
	srv "github.com/lohmander/hostanapp/echo_provider/server"
)

func main() {
	echoServer := &srv.EchoProviderServer{}
	echoServer.Serve(5000)
}
