package netatmo

import (
	"bytes"
	"encoding/json"

	"io/ioutil"
	"net/http"
)

const (
	// DefaultBaseURL is netatmo api url
	baseURL = "https://api.netatmo.net/"
	// DefaultAuthURL is netatmo auth url
	authURL = baseURL + "oauth2/token"
	// DefaultDeviceURL is netatmo device url
	deviceURL = baseURL + "/api/gethomedata"
)

type LoginRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope"`
}
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type Home struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Altitude    int      `json:"altitude"`
	Coordinates []string `json:"coordinates"`
	Country     string   `json:"country"`
	Timezone    string   `json:"timezone"`
	Rooms       []Room   `json:"rooms"`
	Modules     []Module `json:"modules"`
	Persons     []Person `json:"persons"`
}
type HomesDataResponse struct {
	Status     string                  `json:"status"`
	TimeExec   float32                 `json:"time_exec"`
	TimeServer uint32                  `json:"time_server"`
	Body       []HomesDataResponseBody `json:"body"`
}
type HomesDataResponseBody struct {
	Homes []Home `json:"homes"`
	User  User   `json:"user"`
}
type HomeStatusRequest struct {
	HomeID      string `json:"home_id"`
	DeviceTypes string `json:"device_types"`
}

type HomeStatusResponse struct {
	Body HomeStatusResponseBody `json:"body"`
	// Include other fields as necessary based on the API specification.
}
type HomeStatusResponseBody struct {
	Home []Home `json:"home"`
}

type Room struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	ModuleIDs []string `json:"module_ids"`
}

type Module struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	SetupDate     int64    `json:"setup_date"`
	RoomID        string   `json:"room_id,omitempty"`
	ModuleBridged []string `json:"module_bridged,omitempty"`
	Bridge        string   `json:"bridge,omitempty"`
	VpnUrl        string   `json:"vpn_url"`
	IsLocal       string   `json:"is_local"`
}
type Person struct {
	ID     string `json:"id"`
	Pseudo string `json:"pseudo"`
	URL    string `json:"url"`
}

type User struct {
	Email             string `json:"email"`
	Language          string `json:"langage"`
	Locale            string `json:"locale"`
	FeelLikeAlgorithm int    `json:"feel_like_algorithm"`
	UnitPressure      int    `json:"unit_pressure"`
	UnitSystem        int    `json:"unit_system"`
	UnitWind          int    `json:"unit_wind"`
	ID                string `json:"id"`
}

func Login(username string, password string) (*LoginResponse, error) {
	loginRequest := &LoginRequest{
		Username:     username,
		Password:     password,
		GrantType:    "password",
		ClientID:     "Your client id",
		ClientSecret: "Your client secret",
		Scope:        "read_station",
	}

	reqBody, err := json.Marshal(loginRequest)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(baseURL+"/"+authURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var loginResp LoginResponse
	err = json.Unmarshal(body, &loginResp)
	if err != nil {
		return nil, err
	}

	return &loginResp, nil
}
func GetHomesData(accessToken string) (*HomesDataResponse, error) {
	req, err := http.NewRequest("GET", "https://api.netatmo.com/api/homesdata", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var homesDataResp HomesDataResponse
	err = json.Unmarshal(body, &homesDataResp)
	if err != nil {
		return nil, err
	}

	return &homesDataResp, nil
}
func GetHomeStatus(homeID string, accessToken string) (*HomeStatusResponse, error) {
	req, err := http.NewRequest("GET", "https://api.netatmo.com/api/homestatus", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	query := req.URL.Query()
	query.Add("home_id", homeID)

	req.URL.RawQuery = query.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var homeStatusResponse HomeStatusResponse
	err = json.Unmarshal(body, &homeStatusResponse)
	if err != nil {
		return nil, err
	}

	return &homeStatusResponse, nil
}
