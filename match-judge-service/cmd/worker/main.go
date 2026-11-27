package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("match-judge-service Startup successful")
	fmt.Println("REDIS_ADDR =", os.Getenv("REDIS_ADDR"))
	for {
		time.Sleep(10 * time.Second)
		fmt.Println("10 Second loop Test")
	}
}
