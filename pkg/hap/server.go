package hap

import (
	"bufio"
	"crypto/ed25519"
	"github.com/brutella/hap"
	"github.com/brutella/hap/tlv8"
	"io"
	"net"
	"net/http"
)

type Server struct {
	// Pin can't be null because server proof will be wrong
	Pin string `json:"-"`

	ServerID string `json:"server_id"`
	// 32 bytes private key + 32 bytes public key
	ServerPrivate []byte `json:"server_private"`

	// Pairings can be nil for disable pair verify check
	// ClientID: 32 bytes client public + 1 byte (isAdmin)
	Pairings map[string][]byte `json:"pairings"`

	DefaultPlainHandler  func(w io.Writer, r *http.Request) error
	DefaultSecureHandler func(w io.Writer, r *http.Request) error

	OnPairChange func(clientID string, clientPublic []byte) `json:"-"`
	OnRequest    func(w io.Writer, r *http.Request)         `json:"-"`
}

func GenerateKey() []byte {
	_, key, _ := ed25519.GenerateKey(nil)
	return key
}

func NewServer(name string) *Server {
	return &Server{
		ServerID:      GenerateID(name),
		ServerPrivate: GenerateKey(),
		Pairings:      map[string][]byte{},
	}
}

func (s *Server) Serve(address string) (err error) {
	var ln net.Listener
	if ln, err = net.Listen("tcp", address); err != nil {
		return
	}

	for {
		var conn net.Conn
		if conn, err = ln.Accept(); err != nil {
			continue
		}
		go func() {
			//fmt.Printf("[%s] new connection\n", conn.RemoteAddr().String())
			s.Accept(conn)
			//fmt.Printf("[%s] close connection\n", conn.RemoteAddr().String())
		}()
	}
}

func (s *Server) Accept(conn net.Conn) (err error) {
	defer conn.Close()

	var req *http.Request
	r := bufio.NewReader(conn)
	if req, err = http.ReadRequest(r); err != nil {
		return
	}

	return s.HandleRequest(conn, req)
}

func (s *Server) HandleRequest(conn net.Conn, req *http.Request) (err error) {
	if s.OnRequest != nil {
		s.OnRequest(conn, req)
	}

	switch req.URL.Path {
	case UriPairSetup:
		if _, err = s.PairSetupHandler(conn, req); err != nil {
			return
		}

	case UriPairVerify:
		var secure *Secure
		if secure, err = s.PairVerifyHandler(conn, req); err != nil {
			return
		}

		err = s.HandleSecure(secure)

	default:
		if s.DefaultPlainHandler != nil {
			err = s.DefaultPlainHandler(conn, req)
		}
	}

	return
}

func (s *Server) HandleSecure(secure *Secure) (err error) {
	r := bufio.NewReader(secure)
	for {
		var req *http.Request
		if req, err = http.ReadRequest(r); err != nil {
			return
		}

		if s.OnRequest != nil {
			s.OnRequest(secure, req)
		}

		switch req.URL.Path {
		case UriPairings:
			s.HandlePairings(secure, req)
		default:
			if err = s.DefaultSecureHandler(secure, req); err != nil {
				return
			}
		}
	}
}

func (s *Server) HandlePairings(w io.Writer, r *http.Request) {
	req := struct {
		Method     byte   `tlv8:"0"`
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Permission byte   `tlv8:"11"`
		State      byte   `tlv8:"6"`
	}{}

	if err := tlv8.UnmarshalReader(r.Body, &req); err != nil {
		panic(err)
	}

	switch req.Method {
	case hap.MethodAddPairing, hap.MethodDeletePairing:
		res := struct {
			State byte `tlv8:"6"`
		}{
			State: hap.M2,
		}
		data, err := tlv8.Marshal(res)
		if err != nil {
			panic(err)
		}
		if err = WriteResponse(w, http.StatusOK, MimeJSON, data); err != nil {
			panic(err)
		}
	}
}
