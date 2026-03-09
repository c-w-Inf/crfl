package crfl

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

type Pack struct {
	status int
	lid    uint32
	cid    uint32
	dat    []byte
}

func readPack(r io.Reader) (Pack, error) {
	buf := make([]byte, 16)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return Pack{}, err
	} else if n != 16 {
		return Pack{}, errors.New("pack format error")
	}

	tactic := string(buf[:4])
	lid := BytestoU32(buf[4:8])
	cid := BytestoU32(buf[8:12])
	ndat := int(BytestoU32(buf[12:]))

	dat := make([]byte, ndat)
	n, err = io.ReadFull(r, dat)
	if err != nil {
		return Pack{}, err
	} else if n != ndat {
		return Pack{}, errors.New("pack format error")
	}

	var status int
	switch tactic {
	case "conn":
		status = 0
	case "send":
		status = 1
	case "stop":
		status = 2
	case "rejc":
		status = -1
	default:
		return Pack{}, errors.New("pack format error")
	}

	return Pack{
		status: status,
		lid:    lid,
		cid:    cid,
		dat:    dat,
	}, nil
}

func copyPack(w io.Writer, p Pack) error {
	var tactic string
	switch p.status {
	case 0:
		tactic = "conn"
	case 1:
		tactic = "send"
	case 2:
		tactic = "stop"
	case -1:
		tactic = "rejc"
	}

	if err := copyString(w, tactic); err != nil {
		return err
	}
	if err := copyBytes(w, U32toBytes(p.lid)); err != nil {
		return err
	}
	if err := copyBytes(w, U32toBytes(p.cid)); err != nil {
		return err
	}
	if err := copyBytes(w, U32toBytes(uint32(len(p.dat)))); err != nil {
		return err
	}
	return copyBytes(w, p.dat)
}

func (p Pack) String() string {
	return fmt.Sprintf("{stat=%d,lid=%d,cid=%d,datlen=%d}", p.status, p.lid, p.cid, len(p.dat))
}

func askTLSs(conn net.Conn, certs []tls.Certificate) (net.Conn, error) {
	buf := make([]byte, 4)
	n, err := io.ReadFull(conn, buf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect with client: %v", err)
	} else if n != 4 {
		return nil, fmt.Errorf("failed to identify client")
	} else if string(buf) == "crfl" {

		if len(certs) == 0 {
			if err := copyString(conn, "crfl"); err != nil {
				return nil, fmt.Errorf("failed to connect with client: %v", err)
			}
			return conn, nil
		}

		if err := copyString(conn, "ctls"); err != nil {
			return nil, fmt.Errorf("failed to connect with client: %v", err)
		}

		tlsconn := tls.Server(conn, &tls.Config{
			Certificates: certs,
		})

		if err := tlsconn.Handshake(); err != nil {
			return nil, fmt.Errorf("failed to TLS handshake: %v", err)
		}
		return tlsconn, nil

	} else {
		return nil, fmt.Errorf("failed to identify client")
	}
}

func askTLSc(conn net.Conn, name string) (net.Conn, error) {
	buf := make([]byte, 4)

	if err := copyString(conn, "crfl"); err != nil {
		return nil, fmt.Errorf("failed to connect with client: %v", err)
	}

	n, err := io.ReadFull(conn, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to connect with client: %v", err)
	} else if n != 4 {
		return nil, fmt.Errorf("failed to identify client")
	} else if string(buf) == "crfl" {
		return conn, nil
	} else if string(buf) == "ctls" {

		tlsconn := tls.Client(conn, &tls.Config{
			ServerName:         name,
			InsecureSkipVerify: false,
		})
		if err := tlsconn.Handshake(); err != nil {
			return nil, fmt.Errorf("failed to tls handshake: %v", err)
		}
		return tlsconn, nil

	} else {
		return nil, fmt.Errorf("failed to identify client")
	}
}

func copyString(w io.Writer, str string) error {
	_, err := io.Copy(w, bytes.NewReader([]byte(str)))
	return err
}

func copyBytes(w io.Writer, b []byte) error {
	_, err := io.Copy(w, bytes.NewReader(b))
	return err
}

func U32toBytes(v uint32) []byte {
	res := make([]byte, 4)
	binary.BigEndian.PutUint32(res, v)
	return res
}

func BytestoU32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
