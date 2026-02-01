package wyze

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/wyze"
)

type AccountConfig struct {
	APIKey   string `yaml:"api_key"`
	APIID    string `yaml:"api_id"`
	Password string `yaml:"password"`
}

var accounts map[string]AccountConfig

func Init() {
	var v struct {
		Cfg map[string]AccountConfig `yaml:"wyze"`
	}
	app.LoadConfig(&v)

	accounts = v.Cfg

	log := app.GetLogger("wyze")

	streams.HandleFunc("wyze", func(rawURL string) (core.Producer, error) {
		log.Debug().Msgf("wyze: dial %s", rawURL)
		return wyze.NewProducer(rawURL)
	})

	api.HandleFunc("api/wyze", apiWyze)
}

func getCloud(email string) (*wyze.Cloud, error) {
	cfg, ok := accounts[email]
	if !ok {
		return nil, fmt.Errorf("wyze: account not found: %s", email)
	}

	if cfg.APIKey == "" || cfg.APIID == "" {
		return nil, fmt.Errorf("wyze: api_key and api_id required for account: %s", email)
	}

	cloud := wyze.NewCloud(cfg.APIKey, cfg.APIID)

	if err := cloud.Login(email, cfg.Password); err != nil {
		return nil, err
	}

	return cloud, nil
}

func apiWyze(w http.ResponseWriter, r *http.Request) {
	if api.IsReadOnly() && r.Method != "GET" {
		api.ReadOnlyError(w)
		return
	}
	switch r.Method {
	case "GET":
		apiDeviceList(w, r)
	case "POST":
		apiAuth(w, r)
	}
}

func apiDeviceList(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	email := query.Get("id")
	if email == "" {
		accountList := make([]string, 0, len(accounts))
		for id := range accounts {
			accountList = append(accountList, id)
		}
		api.ResponseJSON(w, accountList)
		return
	}

	err := func() error {
		cloud, err := getCloud(email)
		if err != nil {
			return err
		}

		cameras, err := cloud.GetCameraList()
		if err != nil {
			return err
		}

		var items []*api.Source
		for _, cam := range cameras {
			items = append(items, &api.Source{
				Name: cam.Nickname,
				Info: fmt.Sprintf("%s | %s | %s", cam.ProductModel, cam.MAC, cam.IP),
				URL:  buildStreamURL(cam),
			})
		}

		api.ResponseSources(w, items)
		return nil
	}()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func apiAuth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	email := r.Form.Get("email")
	password := r.Form.Get("password")
	apiKey := r.Form.Get("api_key")
	apiID := r.Form.Get("api_id")

	if email == "" || password == "" || apiKey == "" || apiID == "" {
		http.Error(w, "email, password, api_key and api_id required", http.StatusBadRequest)
		return
	}

	// Try to login
	cloud := wyze.NewCloud(apiKey, apiID)

	if err := cloud.Login(email, password); err != nil {
		// Check for MFA error
		var authErr *wyze.AuthError
		if ok := isAuthError(err, &authErr); ok {
			w.Header().Set("Content-Type", api.MimeJSON)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(authErr)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cfg := map[string]string{
		"password": password,
		"api_key":  apiKey,
		"api_id":   apiID,
	}

	if err := app.PatchConfig([]string{"wyze", email}, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if accounts == nil {
		accounts = make(map[string]AccountConfig)
	}
	accounts[email] = AccountConfig{
		APIKey:   apiKey,
		APIID:    apiID,
		Password: password,
	}

	cameras, err := cloud.GetCameraList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*api.Source
	for _, cam := range cameras {
		items = append(items, &api.Source{
			Name: cam.Nickname,
			Info: fmt.Sprintf("%s | %s | %s", cam.ProductModel, cam.MAC, cam.IP),
			URL:  buildStreamURL(cam),
		})
	}

	api.ResponseSources(w, items)
}

func buildStreamURL(cam *wyze.Camera) string {
	query := url.Values{}
	query.Set("uid", cam.P2PID)
	query.Set("enr", cam.ENR)
	query.Set("mac", cam.MAC)
	query.Set("model", cam.ProductModel)

	if cam.DTLS == 1 {
		query.Set("dtls", "true")
	}

	return fmt.Sprintf("wyze://%s?%s", cam.IP, query.Encode())
}

func isAuthError(err error, target **wyze.AuthError) bool {
	if e, ok := err.(*wyze.AuthError); ok {
		*target = e
		return true
	}
	return false
}
