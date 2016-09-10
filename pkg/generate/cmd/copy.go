package main

import (
	"fmt"
	"time"

	"github.com/docker/docker/container"
	"github.com/docker/docker/pkg/generate"
)

func main() {
	ignoreErrs := []interface{}{time.Time{}}
	i, b, err := generate.DeepCopyFunc("o", &container.Container{}, ignoreErrs)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(i))
	fmt.Println(string(b))
}
