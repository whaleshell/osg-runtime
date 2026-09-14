package proxy

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	wsOpcodeContinuation = 0x0
	wsOpcodeText         = 0x1
	wsOpcodeBinary       = 0x2
	wsOpcodeClose        = 0x8
	wsOpcodePing         = 0x9
	wsOpcodePong         = 0xA
)

type wsFrame struct {
	Opcode  byte
	Payload []byte
	Fin     bool
}

func readWSFrame(r io.Reader) (wsFrame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return wsFrame{}, err
	}
	fin := hdr[0]&0x80 != 0
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	n := int(hdr[1] & 0x7f)
	var length uint64
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
		if length > 16<<20 {
			return wsFrame{}, fmt.Errorf("websocket: frame too large")
		}
	default:
		length = uint64(n)
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return wsFrame{}, err
		}
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return wsFrame{}, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return wsFrame{Opcode: opcode, Payload: payload, Fin: fin}, nil
}

func writeWSFrame(w io.Writer, opcode byte, payload []byte, mask bool) error {
	finOpcode := byte(0x80) | (opcode & 0x0f)
	n := len(payload)
	var hdr []byte
	hdr = append(hdr, finOpcode)
	switch {
	case n < 126:
		b := byte(n)
		if mask {
			b |= 0x80
		}
		hdr = append(hdr, b)
	case n <= 65535:
		b := byte(126)
		if mask {
			b |= 0x80
		}
		hdr = append(hdr, b, byte(n>>8), byte(n))
	default:
		b := byte(127)
		if mask {
			b |= 0x80
		}
		hdr = append(hdr, b)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}
	var maskKey [4]byte
	if mask {
		_, _ = rand.Read(maskKey[:])
		hdr = append(hdr, maskKey[:]...)
		out := make([]byte, n)
		for i := 0; i < n; i++ {
			out[i] = payload[i] ^ maskKey[i%4]
		}
		payload = out
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}
