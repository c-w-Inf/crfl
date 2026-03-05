package crfl

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
)

type Normal struct {
	ip            string
	port          int
	listeningPort []int
	send          chan Pack
	recv          []chan Pack
}

func NewNormal(ip string, port int, listeningPort []int) *Normal {
	return &Normal{
		ip:            ip,
		port:          port,
		listeningPort: listeningPort,
		send:          make(chan Pack, 1024),
	}
}

func (n *Normal) Start(verbose bool) error {
	conn, err := net.Dial("tcp", n.ip+":"+strconv.Itoa(n.port))
	if err != nil {
		return fmt.Errorf("failed to connect with server %s:%d: %v", n.ip, n.port, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close connection to server: %v", err)
		}
	}()

	if err := copyBytes(conn, append([]byte("crfl"), U32toBytes(^uint32(0))...)); err != nil {
		return fmt.Errorf("failed to connect with server: %v", err)
	}
	buf := make([]byte, 14)
	nn, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("failed to connect with server: %v", err)
	} else if nn != 14 || string(buf[:10]) != "crflnormal" {
		return fmt.Errorf("failed to identify server")
	}

	nlisten := int(BytestoU32(buf[10:]))
	n.recv = make([]chan Pack, nlisten)
	for i := range n.recv {
		n.recv[i] = make(chan Pack, 1024)
	}

	nports := len(n.listeningPort)
	for lid := 0; lid < nlisten; lid++ {
		if lid < nports {
			go n.listen(n.listeningPort[lid], lid, verbose)
		} else {
			go n.listen(n.listeningPort[nports-1]+lid-nports+1, lid, verbose)
		}
	}

	go func() {
		for {
			pack := <-n.send
			if verbose {
				log.Printf("*sending pack %v to server", pack)
			}
			if err := copyPack(conn, pack); err != nil {
				if verbose {
					log.Printf("*failed to send pack: %v", err)
				}
				break
			}
		}
	}()

	for {
		pack, err := readPack(conn)
		if verbose {
			log.Printf("*received pack %v from server", pack)
		}
		if err != nil {
			return fmt.Errorf("failed to connect with server: %v", err)
		}
		n.recv[pack.lid] <- pack
	}
}

func (n *Normal) listen(port int, lid int, verbose bool) {
	listen, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Printf("failed to listen on %d: %v", port, err)
		return
	}
	defer func() {
		if err := listen.Close(); err != nil {
			log.Printf("failed to close listen on port %d: %v", port, err)
		}
	}()

	acc := make(chan chan Pack, 1024)
	gets := make(map[uint32]chan Pack)

	go func() {
		for {
			pack := <-n.recv[lid]

			switch pack.status {
			case -1: // rejc
				get := make(chan Pack, 1024)
				acc <- get
				get <- pack

			case 0: // conn
				get := make(chan Pack, 1024)
				acc <- get

				gets[pack.cid] = get
				get <- pack

			case 1: // send
				gets[pack.cid] <- pack

			case 2: // stop
				gets[pack.cid] <- pack
				delete(gets, pack.cid)
			}
		}
	}()

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Printf("failed to accept socket on port %d: %v", port, err)
			continue
		}
		go n.handle(conn, lid, acc, verbose)
	}
}

func (n *Normal) handle(conn net.Conn, lid int, acc chan chan Pack, verbose bool) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close local connection with listener %d: %v", lid, err)
		}
	}()
	n.send <- Pack{
		status: 0,
		lid:    uint32(lid),
	}

	get := <-acc
	handshake := <-get
	if handshake.status == -1 {
		if verbose {
			log.Printf("*rejected by remote listener %d", lid)
		}
		return
	}

	cid := handshake.cid
	if verbose {
		log.Printf("*start a new session %d with remote listener %d", cid, lid)
	}

	req := make(chan []byte, 1024)
	go func() {
		for {
			buf := make([]byte, 8192)
			n, err := conn.Read(buf)
			if err != nil {
				if verbose {
					log.Printf("*unable to connect with client in session %d with remote listener %d: %v", cid, lid, err)
				}
				close(req)
				break
			}
			req <- buf[:n]
		}
	}()

loop:
	for {
		select {
		case dat, ok := <-req:
			if !ok {
				break loop
			}
			n.send <- Pack{
				status: 1,
				lid:    uint32(lid),
				cid:    cid,
				dat:    dat,
			}

		case pack := <-get:
			switch pack.status {
			case 1: // send
				if err := copyBytes(conn, pack.dat); err != nil {
					log.Printf("*unable to connect with client in session %d with remote listener %d: %v", cid, lid, err)
					break loop
				}

			case 2: // stop
				return
			}
		}
	}

	n.send <- Pack{
		status: 2,
		lid:    uint32(lid),
		cid:    cid,
	}
	if verbose {
		log.Printf("*session %d d with remote listener %d", cid, lid)
	}
}
