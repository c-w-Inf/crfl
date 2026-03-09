package crfl

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

type Listener struct {
	ip          string
	port        int
	forwardPort int
	lid         int
	handlers    map[uint32]chan Pack
	send        chan Pack
	mu          sync.RWMutex
}

func NewListener(ip string, port int, forwardPort int, lid int) *Listener {
	return &Listener{
		ip:          ip,
		port:        port,
		forwardPort: forwardPort,
		lid:         lid,
		handlers:    make(map[uint32]chan Pack),
		send:        make(chan Pack, 1024),
	}
}

func (l *Listener) Start(verbose bool) error {
	rconn, err := net.Dial("tcp", l.ip+":"+strconv.Itoa(l.port))
	if err != nil {
		return fmt.Errorf("failed to connect with server %s:%d: %v", l.ip, l.port, err)
	}
	defer func() {
		if err := rconn.Close(); err != nil {
			log.Printf("failed to close connection to server: %v", err)
		}
	}()

	conn, err := askTLSc(rconn, l.ip)
	if err != nil {
		return err
	}

	if err := copyBytes(conn, U32toBytes(uint32(l.lid))); err != nil {
		return fmt.Errorf("failed to connect with server: %v", err)
	}
	buf := make([]byte, 10)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("failed to connect with server: %v", err)
	} else if string(buf[:n]) == "crfloccupy" {
		return fmt.Errorf("listening id has been occupied")
	} else if string(buf[:n]) == "crfloutofb" {
		return fmt.Errorf("listening id is out of bounds")
	} else if string(buf[:n]) != "crfllisten" {
		return fmt.Errorf("failed to identify server")
	}

	go func() {
		for {
			pack := <-l.send
			if verbose {
				log.Printf("*sending pack %v to server", pack)
			}
			if err := copyPack(conn, pack); err != nil {
				if verbose {
					log.Printf("*unable to send pack to server: %v", err)
				}
				break
			}
		}
	}()

	var nxtid uint32 = 0
	for {
		pack, err := readPack(conn)
		if verbose {
			log.Printf("*receive pack %v from server", pack)
		}
		if err != nil {
			return fmt.Errorf("failed to connect with server: %v", err)
		}

		switch pack.status {
		case 0: // conn
			l.mu.RLock()
			for {
				nxtid++
				if _, ok := l.handlers[nxtid]; !ok {
					break
				}
			}
			hand := make(chan Pack, 1024)
			l.handlers[nxtid] = hand
			l.mu.RUnlock()
			if verbose {
				log.Printf("*start a new session %d", nxtid)
			}

			go l.handle(hand, nxtid, verbose)

		default:
			l.mu.Lock()
			if hand, ok := l.handlers[pack.cid]; ok {
				hand <- pack
			}
			l.mu.Unlock()
		}
	}
}

func (l *Listener) handle(hand chan Pack, cid uint32, verbose bool) {
	defer func() {
		l.mu.Lock()
		delete(l.handlers, cid)
		l.mu.Unlock()
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(l.forwardPort))
	if err != nil {
		if verbose {
			log.Printf("*unable to connect with listener in session %d: %v", cid, err)
		}
		l.send <- Pack{
			status: -1,
		}
		return
	}

	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close local connection in session %d: %v", cid, err)
		}
	}()
	l.send <- Pack{
		status: 0,
		cid:    cid,
	}

	resp := make(chan []byte, 1024)
	go func() {
		for {
			buf := make([]byte, 8192)
			n, err := conn.Read(buf)
			if err != nil {
				if verbose {
					log.Printf("*unable to connect with listener in session %d: %v", cid, err)
				}
				close(resp)
				break
			}
			resp <- buf[:n]
		}
	}()

loop:
	for {
		select {
		case dat, ok := <-resp:
			if !ok {
				break loop
			}

			l.send <- Pack{
				status: 1,
				cid:    cid,
				dat:    dat,
			}

		case pack := <-hand:
			switch pack.status {
			case 1: // send
				if err := copyBytes(conn, pack.dat); err != nil {
					if verbose {
						log.Printf("*failed to connect with listener in session %d: %v", cid, err)
					}
					break loop
				}

			case 2: // stop
				return
			}
		}
	}

	l.send <- Pack{
		status: 2,
		cid:    cid,
	}

	if verbose {
		log.Printf("*session %d ended", cid)
	}
}
