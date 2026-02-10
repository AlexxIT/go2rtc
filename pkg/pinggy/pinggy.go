package pinggy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	SSH *ssh.Client
	TCP net.Listener
	API *http.Client
}

func NewClient(proto string) (*Client, error) {
	switch proto {
	case "http", "tcp", "tls", "tlstcp":
	case "":
		proto = "http"
	default:
		return nil, errors.New("pinggy: unsupported proto: " + proto)
	}

	config := &ssh.ClientConfig{
		User:            "auth+" + proto,
		Auth:            []ssh.AuthMethod{ssh.Password("nopass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	client, err := ssh.Dial("tcp", "a.pinggy.io:443", config)
	if err != nil {
		return nil, err
	}

	ln, err := client.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	c := &Client{
		SSH: client,
		TCP: ln,
		API: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return client.Dial(network, addr)
				},
			},
		},
	}

	if proto == "http" {
		if err = c.NewSession(); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return c, nil
}

func (c *Client) Close() error {
	return errors.Join(c.SSH.Close(), c.TCP.Close())
}

func (c *Client) NewSession() error {
	session, err := c.SSH.NewSession()
	if err != nil {
		return err
	}
	return session.Shell()
}

func (c *Client) GetURLs() ([]string, error) {
	res, err := c.API.Get("http://localhost:4300/urls")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var v struct {
		URLs []string `json:"urls"`
	}

	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return nil, err
	}

	return v.URLs, nil
}

func (c *Client) Proxy(address string) error {
	defer c.TCP.Close()

	for {
		conn, err := c.TCP.Accept()
		if err != nil {
			return err
		}
		go proxy(conn, address)
	}
}

func proxy(conn1 net.Conn, address string) {
	defer conn1.Close()

	conn2, err := net.Dial("tcp", address)
	if err != nil {
		return
	}
	defer conn2.Close()

	go io.Copy(conn2, conn1)
	io.Copy(conn1, conn2)
}

// DialTLS like ssh.Dial but with TLS
//func DialTLS(network, addr, sni string, config *ssh.ClientConfig) (*ssh.Client, error) {
//	conn, err := net.DialTimeout(network, addr, config.Timeout)
//	if err != nil {
//		return nil, err
//	}
//	conn = tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: sni == ""})
//	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
//	if err != nil {
//		return nil, err
//	}
//	return ssh.NewClient(c, chans, reqs), nil
//}
