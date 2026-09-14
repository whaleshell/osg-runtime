// Command osg-sshd is a minimal SSH server for sandbox connect --ssh.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/zorneth/osg-core/defaults"
	"golang.org/x/crypto/ssh"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	listen := fmt.Sprintf("%s:%d", defaults.ProxyListenHost, defaults.GuestSSHPort)
	authKeys := defaults.GuestSSHAuthorizedKeys
	hostKeyPath := defaults.GuestSSHHostKey
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--listen":
			i++
			listen = args[i]
		case "--authorized-keys":
			i++
			authKeys = args[i]
		case "--host-key":
			i++
			hostKeyPath = args[i]
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "usage: osg-sshd [--listen ADDR] [--authorized-keys FILE] [--host-key FILE]\n")
			return nil
		default:
			return fmt.Errorf("unknown flag %q", args[i])
		}
	}
	pubKeys, err := loadAuthorized(authKeys)
	if err != nil {
		return err
	}
	signer, err := loadOrCreateHostKey(hostKeyPath)
	if err != nil {
		return err
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			for _, k := range pubKeys {
				if ssh.FingerprintSHA256(k) == ssh.FingerprintSHA256(key) {
					return &ssh.Permissions{}, nil
				}
			}
			return nil, fmt.Errorf("unknown key")
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "osg-sshd: listening on %s\n", listen)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(c, cfg)
	}
}

func handleConn(c net.Conn, cfg *ssh.ServerConfig) {
	defer c.Close()
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unknown")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for req := range requests {
				switch req.Type {
				case "pty-req", "shell", "env":
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
					if req.Type == "shell" {
						cmd := exec.Command("bash", "-l")
						cmd.Stdin = ch
						cmd.Stdout = ch
						cmd.Stderr = ch
						_ = cmd.Run()
						return
					}
				case "exec":
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
					payload := string(req.Payload)
					// SSH exec payload: uint32 length + command
					if len(req.Payload) >= 4 {
						n := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
						if 4+n <= len(req.Payload) {
							payload = string(req.Payload[4 : 4+n])
						}
					}
					cmd := exec.Command("sh", "-c", payload)
					cmd.Stdin = ch
					cmd.Stdout = ch
					cmd.Stderr = ch
					_ = cmd.Run()
					return
				default:
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
				}
			}
		}()
	}
}

func loadAuthorized(path string) ([]ssh.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var keys []ssh.PublicKey
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		keys = append(keys, pk)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no authorized keys in %s", path)
	}
	return keys, nil
}

func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(b)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	// Marshal OpenSSH private key
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(pemBytes)
}

func filepathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "."
	}
	return p[:i]
}
