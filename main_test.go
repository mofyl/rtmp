package main

import (
	"fmt"
	"testing"
)

func TestSlice(t *testing.T) {

	b := make([]int, 5)
	b[0] = 1
	b[1] = 2
	b[2] = 3
	b[3] = 4
	b[4] = 5

	//b = append(b, 1)
	//b = append(b, 2)
	//b = append(b, 3)
	//b = append(b, 4)
	//b = append(b, 5)

	//c := b[1:3:10]
	c := b[1:4]
	b[3] = 10
	//b[2] = 10
	//fmt.Println(data)
	//c = append(c, 11)
	//c = append(c, 12)
	//c = append(c, 13)
	fmt.Println(b)
	fmt.Println(c)
	//
	//hdr := (*reflect.SliceHeader)(unsafe.Pointer(&c))
	//data := *(*[4]int)(unsafe.Pointer(hdr.Data))
	//fmt.Println(data)
	b = append(b, 23)
	fmt.Println(b)
	fmt.Println(c)
	//
	//hdr1 := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	//data1 := *(*[5]int)(unsafe.Pointer(hdr1.Data))
	//fmt.Println(data1)
}
