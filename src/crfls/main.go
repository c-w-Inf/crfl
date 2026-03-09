package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"strings"

	"crfl/src/crfl"
)

func main() {
	port := flag.Int("p", 3862, "port to listen on")
	maxListeners := flag.Int("n", 1, "max listeners")
	verbose := flag.Bool("v", false, "verbose mode")
	certPath := flag.String("s", "", "certification file paths in format of cert:key;...")
	flag.Parse()

	certs, err := func(arg string) ([]tls.Certificate, error) {
		if arg == "" {
			return []tls.Certificate{}, nil
		} else {
			parts := strings.Split(arg, ",")
			certs := make([]tls.Certificate, len(parts))
			for i, part := range parts {
				paths := strings.Split(part, ":")
				if len(paths) != 2 {
					return nil, fmt.Errorf("there should be 2 paths for each certificate")
				}

				cert, err := tls.LoadX509KeyPair(paths[0], paths[1])
				if err != nil {
					return nil, err
				}
				certs[i] = cert
			}
			return certs, nil
		}
	}(*certPath)
	if err != nil {
		fmt.Printf("-s parsing error: %v\n", err)
		return
	}

	if err := crfl.NewServer(*port, *maxListeners).Start(certs, *verbose); err != nil {
		fmt.Printf("crfls stopped: %v\n", err)
	}
}
