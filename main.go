package main

import (
	"fmt"
	"net"
)

func main(){

	l , err := net.Listen("tcp4" , "127.0.0.1:9000")

	if err != nil {
		fmt.Println("listen err is " , err.Error())
		return
	}
	fmt.Println("Net is Listen Addr is " , l.Addr().String())
	for {
		conn , err := l.Accept()
		if err != nil {
			fmt.Println("Accept Err is " , err.Error())
			continue
		}
		fmt.Println("Success")
		err = handshake(conn)

		if err != nil {
			fmt.Println("err is " , err.Error())
			conn.Close()
			break
		}
	}

	l.Close()
}