package gopro

import (
	"net"
	"net/http"
	"regexp"
)

func Discovery() (urls []string) {
	ints, err := net.Interfaces()
	if err != nil {
		return nil
	}

	// The socket address for USB connections is 172.2X.1YZ.51:8080
	// https://gopro.github.io/OpenGoPro/http_2_0#socket-address
	re := regexp.MustCompile(`^172\.2\d\.1\d\d\.`)

	for _, itf := range ints {
		addrs, err := itf.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			host := addr.String()
			if !re.MatchString(host) {
				continue
			}

			host = host[:11] + "51" // 172.2x.1xx.xxx
			res, err := http.Get("http://" + host + ":8080/gopro/webcam/status")
			if err != nil {
				continue
			}
			_ = res.Body.Close()

			urls = append(urls, host)
		}
	}

	return
}
