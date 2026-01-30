package main

func main() {
	// create http server using std library
	CreateHTTPServer()

	// create the http server using fiber framework
	CreateFiberServer()

	// create http server using echo framework
	CreateEchoServer()
}