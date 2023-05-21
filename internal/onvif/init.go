package onvif

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/rs/zerolog"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func Init() {
	log = app.GetLogger("onvif")

	streams.HandleFunc("onvif", streamOnvif)

	// ONVIF server on all suburls
	api.HandleFunc("/onvif/", onvifDeviceService)

	// ONVIF client autodiscovery
	api.HandleFunc("api/onvif", apiOnvif)
}

var log zerolog.Logger

func streamOnvif(rawURL string) (core.Producer, error) {
	client, err := onvif.NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	uri, err := client.GetURI()
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("[onvif] new uri=%s", uri)

	return streams.GetProducer(uri)
}

func onvifDeviceService(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	action := onvif.GetRequestAction(b)
	if action == "" {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return
	}

	log.Trace().Msgf("[onvif] %s", action)

	var res string

	switch action {
	case onvif.ActionGetCapabilities:
		// important for Hass: Media section
		res = onvif.GetCapabilitiesResponse(r.Host)

	case onvif.ActionGetSystemDateAndTime:
		// important for Hass
		res = onvif.GetSystemDateAndTimeResponse()

	case onvif.ActionGetNetworkInterfaces:
		// important for Hass: none
		res = onvif.GetNetworkInterfacesResponse()

	case onvif.ActionGetDeviceInformation:
		// important for Hass: SerialNumber (unique server ID)
		res = onvif.GetDeviceInformationResponse("", "go2rtc", app.Version, r.Host)

	case onvif.ActionGetServiceCapabilities:
		// important for Hass
		res = onvif.GetServiceCapabilitiesResponse()

	case onvif.ActionSystemReboot:
		res = onvif.SystemRebootResponse()

		time.AfterFunc(time.Second, func() {
			os.Exit(0)
		})

	case onvif.ActionGetProfiles:
		// important for Hass: H264 codec, width, height
		res = onvif.GetProfilesResponse(streams.GetAll())

	case onvif.ActionGetStreamUri:
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		uri := "rtsp://" + host + ":" + rtsp.Port + "/" + onvif.FindTagValue(b, "ProfileToken")
		res = onvif.GetStreamUriResponse(uri)

	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
		log.Debug().Msgf("[onvif] unsupported request:\n%s", b)
		return
	}

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if _, err = w.Write([]byte(res)); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func apiOnvif(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")

	var items []api.Stream

	if src == "" {
		urls, err := onvif.DiscoveryStreamingURLs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, rawURL := range urls {
			u, err := url.Parse(rawURL)
			if err != nil {
				log.Warn().Str("url", rawURL).Msg("[onvif] broken")
				continue
			}

			if u.Scheme != "http" {
				log.Warn().Str("url", rawURL).Msg("[onvif] unsupported")
				continue
			}

			u.Scheme = "onvif"
			u.User = url.UserPassword("user", "pass")

			if u.Path == onvif.PathDevice {
				u.Path = ""
			}

			items = append(items, api.Stream{Name: u.Host, URL: u.String()})
		}
	} else {
		client, err := onvif.NewClient(src)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if l := log.Trace(); l.Enabled() {
			b, _ := client.GetProfiles()
			l.Msgf("[onvif] src=%s profiles:\n%s", src, b)
		}

		name, err := client.GetName()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		tokens, err := client.GetProfilesTokens()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for i, token := range tokens {
			items = append(items, api.Stream{
				Name: name + " stream" + strconv.Itoa(i),
				URL:  src + "?subtype=" + token,
			})
		}

		if len(tokens) > 0 && client.HasSnapshots() {
			items = append(items, api.Stream{
				Name: name + " snapshot",
				URL:  src + "?subtype=" + tokens[0] + "&snapshot",
			})
		}
	}

	api.ResponseStreams(w, items)
}
