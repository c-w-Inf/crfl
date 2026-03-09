package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"crfl/src/crfl"
)

func main() {
	ip := flag.String("s", "127.0.0.1", "server ip")
	port := flag.Int("p", 3862, "port to listen on")
	forwardPort := flag.Int("f", -1, "forwarding port")
	lid := flag.Int("i", -2, "listening port id")
	listeningPorts := flag.String("l", "", "listening ports")
	verbose := flag.Bool("v", false, "verbose mode")
	flag.Parse()

	if *lid < 0 {
		*lid = -2
	}

	if *forwardPort != -1 {
		if *listeningPorts != "" {
			fmt.Printf("-f and -l cannot be simultaneously used\n")
			return
		}

		if err := crfl.NewListener(*ip, *port, *forwardPort, *lid).Start(*verbose); err != nil {
			fmt.Printf("error: %v\n", err)
		}
	} else {
		parts := strings.Split(*listeningPorts, ",")
		ports := make([]int, len(parts))
		for i, part := range parts {
			port, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				fmt.Printf("-l parsing error: %v\n", err)
				return
			}
			ports[i] = port
		}

		if err := crfl.NewNormal(*ip, *port, ports).Start(*verbose); err != nil {
			fmt.Printf("crflc stopped: %v\n", err)
		}
	}
}
