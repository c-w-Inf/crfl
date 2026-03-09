package crfl

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

type Server struct {
	port     int
	listener []*ClientInfo
	mu       sync.RWMutex
}

type ClientInfo struct {
	request  chan Pack
	callback chan chan Pack
}

func NewServer(port int, maxListeners int) *Server {
	return &Server{
		port:     port,
		listener: make([]*ClientInfo, maxListeners),
	}
}

func (s *Server) Start(certs []tls.Certificate, verbose bool) error {
	listen, err := net.Listen("tcp", ":"+strconv.Itoa(s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", s.port, err)
	}
	defer func() {
		if err := listen.Close(); err != nil {
			log.Printf("failed to close listen: %v", err)
		}
	}()

	log.Printf("crfls listening on port %d with %d max listeners", s.port, len(s.listener))

	for {
		rconn, err := listen.Accept()
		if err != nil {
			log.Printf("failed to accept socket: %v", err)
			continue
		}

		conn, err := askTLSs(rconn, certs)
		if err != nil {
			if err := rconn.Close(); err != nil {
				log.Printf("%v", err)
			}
			continue
		}

		go s.handle(conn, verbose)
	}
}

func (s *Server) handle(conn net.Conn, verbose bool) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close connection: %v", err)
		}
	}()

	buf := make([]byte, 4)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		log.Printf("failed to connect with client: %v", err)
		return
	} else if n != 4 {
		log.Printf("failed to identify client")
		return
	}

	id := int(int32(BytestoU32(buf)))
	if id > len(s.listener) {
		log.Printf("client specified an out-of-bounds id: %d", id)
		if err := copyString(conn, "crfloutofb"); err != nil {
			log.Printf("failed to send verification to client: %v", err)
		}
		return
	}
	s.mu.Lock()

	if id <= -2 { // listener; choose any spare id
		id = 0
		for i, l := range s.listener {
			if l == nil {
				id = i
				break
			}
		}
	}
	if id == -1 { // normal

		s.mu.Unlock()
		addr := conn.RemoteAddr().String()
		log.Printf("%s: connected as normal client", addr)
		if err := copyBytes(conn, append([]byte("crflnormal"), U32toBytes(uint32(len(s.listener)))...)); err != nil {
			log.Printf("failed to send verification to client: %v", err)
			return
		}

		send := make(chan Pack, 1024)

		go func() {
			for {
				pack := <-send
				if verbose {
					log.Printf("*%s: sending pack %v", addr, pack)
				}
				if err := copyPack(conn, pack); err != nil {
					if verbose {
						log.Printf("*%s: unable to send pack: %v", addr, err)
					}
					break
				}
			}
		}()

		connids := make([]map[uint32]struct{}, len(s.listener))
		for i := range connids {
			connids[i] = make(map[uint32]struct{})
		}

		for {
			pack, err := readPack(conn)
			if err != nil {
				log.Printf("%s: failed to connect with client: %v", addr, err)
				break
			}
			if verbose {
				log.Printf("*%s: receive pack from normal %v", addr, pack)
			}

			s.mu.RLock()
			listener := s.listener[pack.lid]
			s.mu.RUnlock()

			if listener == nil {
				if pack.status == 0 {
					if verbose {
						log.Printf("*%s: try to connect to unestablished listener %d", addr, pack.lid)
					}
					send <- Pack{
						status: -1,
					}
				} else if verbose {
					log.Printf("*%s: try to connect to closed listener %d", addr, pack.lid)
				}
				continue
			}

			switch pack.status {
			case 0:
				connids[pack.lid][pack.cid] = struct{}{}
				listener.callback <- send
			case 2:
				delete(connids[pack.lid], pack.cid)
			}

			listener.request <- pack
		}

		log.Printf("%s: connection closed", addr)
		s.mu.RLock()
		for lid, lconns := range connids {
			for cid := range lconns {
				s.listener[lid].request <- Pack{
					status: 2,
					lid:    uint32(lid),
					cid:    cid,
				}
			}
		}
		s.mu.RUnlock()

	} else { // listener; specified id

		ci := ClientInfo{
			request:  make(chan Pack, 1024),
			callback: make(chan chan Pack, 1024),
		}

		if s.listener[id] == nil {
			s.listener[id] = &ci
		} else {
			log.Printf("client specified an occupied id: %d", id)
			if err := copyString(conn, "crfloccupy"); err != nil {
				log.Printf("failed to send verification to client: %v", err)
			}
			return
		}
		s.mu.Unlock()
		defer func() {
			s.mu.RLock()
			s.listener[id] = nil
			s.mu.RUnlock()
		}()
		addr := conn.RemoteAddr().String()

		if err := copyString(conn, "crfllisten"); err != nil {
			log.Printf("failed to send verification to client: %v", err)
			return
		}
		log.Printf("%s: connected as listening client on id: %d", conn.RemoteAddr().String(), id)

		callbacks := make(map[uint32]chan Pack)
		send := make(chan Pack, 1024)
		recv := make(chan Pack, 1024)

		go func() {
			for {
				pack := <-send
				if verbose {
					log.Printf("*%s: sending pack %v", addr, pack)
				}
				if err := copyPack(conn, pack); err != nil {
					if verbose {
						log.Printf("*%s: unable to send pack: %v", addr, err)
					}
					break
				}
			}
		}()

		go func() {
			for {
				pack, err := readPack(conn)
				if err != nil {
					if verbose {
						log.Printf("*%s: unable to receive pack: %v", addr, err)
					}
					close(recv)
					break
				}
				if verbose {
					log.Printf("*%s: receive pack from listener %v", addr, pack)
				}
				recv <- pack
			}
		}()

	loop:
		for {
			select {
			case pack, ok := <-recv:
				if !ok {
					break loop
				}
				pack.lid = uint32(id)

				switch pack.status {
				case -1: // rejc
					cb := <-ci.callback
					cb <- pack

				case 0: // conn
					cb := <-ci.callback
					callbacks[pack.cid] = cb
					cb <- pack

				case 1: // send
					cb, ok := callbacks[pack.cid]
					if !ok {
						if verbose {
							log.Printf("*%s: interacting with non-existent id", addr)
						}
						continue
					}
					cb <- pack

				case 2: // stop
					cb, ok := callbacks[pack.cid]
					if ok {
						cb <- pack
					}
				}

			case pack := <-ci.request:
				if pack.status == 2 {
					delete(callbacks, pack.cid)
				}
				send <- pack
			}
		}

		log.Printf("%s: listening closed", addr)
		for cid, cb := range callbacks {
			cb <- Pack{
				status: 2,
				lid:    uint32(id),
				cid:    cid,
			}
		}
	}
}
