package onvif

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("onvif")

	var cfg struct {
		Mod struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"onvif"`
	}
	app.LoadConfig(&cfg)
	username = cfg.Mod.Username
	password = cfg.Mod.Password

	streams.HandleFunc("onvif", streamOnvif)

	// ONVIF server on all suburls
	api.HandleFunc("/onvif/", onvifDeviceService)

	// ONVIF client autodiscovery
	api.HandleFunc("api/onvif", apiOnvif)
}

var (
	log      zerolog.Logger
	username string
	password string
)

func streamOnvif(rawURL string) (core.Producer, error) {
	client, err := onvif.NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	uri, err := client.GetURI()
	if err != nil {
		return nil, err
	}

	// Append hash-based arguments to the retrieved URI
	if i := strings.IndexByte(rawURL, '#'); i > 0 {
		uri += rawURL[i:]
	}

	log.Debug().Msgf("[onvif] new uri=%s", uri)

	if err = streams.Validate(uri); err != nil {
		return nil, err
	}

	return streams.GetProducer(uri)
}

func onvifDeviceService(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	operation := onvif.GetRequestAction(b)
	if operation == "" {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return
	}

	log.Trace().
		Str("remote", r.RemoteAddr).
		Str("user_agent", r.Header.Get("User-Agent")).
		Str("op", operation).
		Msgf("[onvif] server request %s %s:\n%s", r.Method, r.RequestURI, b)

	if username != "" && operation != onvif.DeviceGetSystemDateAndTime {
		if !onvif.VerifyUsernameToken(b, username, password) {
			log.Warn().Str("remote", r.RemoteAddr).Msg("[onvif] unauthorized")
			w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(onvif.NotAuthorizedResponse())
			return
		}
	}

	switch operation {
	case onvif.ServiceGetServiceCapabilities, // important for Hass; routed by URL path below
		onvif.DeviceGetNetworkInterfaces, // important for Hass
		onvif.DeviceGetSystemDateAndTime, // important for Hass
		onvif.DeviceSetSystemDateAndTime, // return just OK
		onvif.DeviceGetDiscoveryMode,
		onvif.DeviceGetDNS,
		onvif.DeviceGetHostname,
		onvif.DeviceGetNetworkDefaultGateway,
		onvif.DeviceGetNetworkProtocols,
		onvif.DeviceGetNTP,
		onvif.DeviceGetScopes,
		onvif.MediaGetVideoEncoderConfiguration,
		onvif.MediaGetVideoEncoderConfigurations,
		onvif.MediaGetAudioEncoderConfiguration,
		onvif.MediaGetAudioEncoderConfigurations,
		onvif.MediaGetVideoEncoderConfigurationOptions,
		onvif.MediaGetAudioSources,
		onvif.MediaGetAudioSourceConfiguration,
		onvif.MediaGetAudioSourceConfigurations,
		onvif.ImagingGetImagingSettings,
		onvif.ImagingGetOptions,
		onvif.ImagingGetMoveOptions,
		onvif.ImagingGetStatus:
		if operation == onvif.ServiceGetServiceCapabilities &&
			strings.Contains(r.URL.Path, "imaging_service") {
			b = onvif.GetImagingServiceCapabilitiesResponse()
		} else if operation == onvif.PTZGetStatus &&
			strings.Contains(r.URL.Path, "ptz_service") {
			b = onvif.GetPTZStatusResponse()
		} else {
			b = onvif.StaticResponse(operation)
		}

	case onvif.PTZGetConfigurations:
		if strings.Contains(r.URL.Path, "ptz_service") {
			b = onvif.GetPTZConfigurationsResponse()
		} else {
			b = onvif.StaticResponse(operation)
		}

	case onvif.PTZGetNodes:
		b = onvif.GetPTZNodesResponse()

	case onvif.PTZAbsoluteMove, onvif.PTZContinuousMove:
		name := onvif.FindTagValue(b, "ProfileToken")
		pan, _ := strconv.ParseFloat(onvif.FindTagAttribute(b, "PanTilt", "x"), 64)
		tilt, _ := strconv.ParseFloat(onvif.FindTagAttribute(b, "PanTilt", "y"), 64)
		stream := streams.Get(name)
		if stream == nil {
			http.Error(w, "unknown profile", http.StatusBadRequest)
			return
		}
		if err = stream.Move(pan, tilt); err != nil {
			log.Warn().Err(err).Str("stream", name).Msg("[onvif] PTZ move")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if operation == onvif.PTZAbsoluteMove {
			b = onvif.GetPTZAbsoluteMoveResponse()
		} else {
			b = onvif.GetPTZContinuousMoveResponse()
		}

	case onvif.PTZStop:
		b = onvif.GetPTZStopResponse()

	case onvif.DeviceGetCapabilities:
		// important for Hass: Media section
		b = onvif.GetCapabilitiesResponse(r.Host)

	case onvif.DeviceGetServices:
		b = onvif.GetServicesResponse(r.Host)

	case onvif.DeviceGetDeviceInformation:
		// important for Hass: SerialNumber (unique server ID)
		b = onvif.GetDeviceInformationResponse("", "go2rtc", app.Version, r.Host)

	case onvif.DeviceSystemReboot:
		b = onvif.StaticResponse(operation)

		time.AfterFunc(time.Second, func() {
			os.Exit(0)
		})

	case onvif.MediaGetVideoSources:
		b = onvif.GetVideoSourcesResponse(streams.GetAllNames())

	case onvif.MediaGetProfiles:
		// important for Hass: H264 codec, width, height
		b = onvif.GetProfilesResponse(streams.GetAllNames())

	case onvif.MediaGetProfile:
		token := onvif.FindTagValue(b, "ProfileToken")
		b = onvif.GetProfileResponse(token)

	case onvif.MediaGetVideoSourceConfigurations:
		// important for Happytime Onvif Client
		b = onvif.GetVideoSourceConfigurationsResponse(streams.GetAllNames())

	case onvif.MediaGetVideoSourceConfiguration:
		token := onvif.FindTagValue(b, "ConfigurationToken")
		b = onvif.GetVideoSourceConfigurationResponse(token)

	case onvif.MediaGetStreamUri:
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host // in case of Host without port
		}

		uri := "rtsp://" + host + ":" + rtsp.Port + "/" + onvif.FindTagValue(b, "ProfileToken")
		b = onvif.GetStreamUriResponse(uri)

	case onvif.MediaGetSnapshotUri:
		uri := "http://" + r.Host + "/api/frame.jpeg?src=" + onvif.FindTagValue(b, "ProfileToken")
		b = onvif.GetSnapshotUriResponse(uri)

	default:
		http.Error(w, "unsupported operation", http.StatusBadRequest)
		log.Warn().
			Str("op", operation).
			Str("remote", r.RemoteAddr).
			Str("user_agent", r.Header.Get("User-Agent")).
			Msg("[onvif] unsupported operation")
		log.Debug().Msgf("[onvif] unsupported request:\n%s", b)
		return
	}

	log.Trace().Msgf("[onvif] server response:\n%s", b)

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if _, err = w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func apiOnvif(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")

	var items []*api.Source

	if src == "" {
		devices, err := onvif.DiscoveryStreamingDevices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, device := range devices {
			u, err := url.Parse(device.URL)
			if err != nil {
				log.Warn().Str("url", device.URL).Msg("[onvif] broken")
				continue
			}

			if u.Scheme != "http" {
				log.Warn().Str("url", device.URL).Msg("[onvif] unsupported")
				continue
			}

			u.Scheme = "onvif"
			u.User = url.UserPassword("user", "pass")

			if u.Path == onvif.PathDevice {
				u.Path = ""
			}

			items = append(items, &api.Source{
				Name: u.Host,
				URL:  u.String(),
				Info: device.Name + " " + device.Hardware,
			})
		}
	} else {
		client, err := onvif.NewClient(src)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if l := log.Trace(); l.Enabled() {
			b, _ := client.MediaRequest(onvif.MediaGetProfiles)
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
			items = append(items, &api.Source{
				Name: name + " stream" + strconv.Itoa(i),
				URL:  src + "?subtype=" + token,
			})
		}

		if len(tokens) > 0 && client.HasSnapshots() {
			items = append(items, &api.Source{
				Name: name + " snapshot",
				URL:  src + "?subtype=" + tokens[0] + "&snapshot",
			})
		}
	}

	api.ResponseSources(w, items)
}
