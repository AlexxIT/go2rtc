package hass

import (
	"errors"
	"github.com/gorilla/websocket"
	"os"
)

type API struct {
	ws *websocket.Conn
}

func NewAPI(url, token string) (*API, error) {
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	api := &API{ws: ws}
	if err = api.Auth(token); err != nil {
		_ = ws.Close()
		return nil, err
	}

	return api, nil
}

func (a *API) Auth(token string) error {
	var res ResponseAuth

	if err := a.ws.ReadJSON(&res); err != nil {
		return err
	}
	if res.Type != "auth_required" {
		return errors.New("hass: wrong type: " + res.Type)
	}

	s := `{"type":"auth","access_token":"` + token + `"}`
	if err := a.ws.WriteMessage(websocket.TextMessage, []byte(s)); err != nil {
		return err
	}
	if err := a.ws.ReadJSON(&res); err != nil {
		return err
	}
	if res.Type != "auth_ok" {
		return errors.New("hass: wrong type: " + res.Type)
	}

	return nil
}

func (a *API) Close() error {
	return a.ws.Close()
}

func (a *API) ExchangeSDP(entityID, offer string) (string, error) {
	var msg = map[string]any{
		"id":        1,
		"type":      "camera/web_rtc_offer",
		"entity_id": entityID,
		"offer":     offer,
	}
	if err := a.ws.WriteJSON(msg); err != nil {
		return "", err
	}

	var res ResponseOffer
	if err := a.ws.ReadJSON(&res); err != nil {
		return "", err
	}

	if res.Type != "result" || !res.Success {
		return "", errors.New("hass: wrong response")
	}

	return res.Result.Answer, nil
}

func (a *API) GetWebRTCEntities() (map[string]string, error) {
	s := `{"id":1,"type":"get_states"}`
	if err := a.ws.WriteMessage(websocket.TextMessage, []byte(s)); err != nil {
		return nil, err
	}

	var res ResponseStates
	if err := a.ws.ReadJSON(&res); err != nil {
		return nil, err
	}
	if res.Type != "result" || !res.Success {
		return nil, errors.New("hass: wrong response")
	}

	entities := map[string]string{}

	for _, entity := range res.Result {
		if entity.Attributes.FrontendStreamType == "web_rtc" {
			entities[entity.Attributes.FriendlyName] = entity.EntityId
		}
	}

	return entities, nil
}

type ResponseAuth struct {
	Type string `json:"type"`
}

type ResponseStates struct {
	//Id      int    `json:"id"`
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Result  []struct {
		EntityId string `json:"entity_id"`
		//State      string `json:"state"`
		Attributes struct {
			//ModelName          string `json:"model_name"`
			//Brand              string `json:"brand"`
			FrontendStreamType string `json:"frontend_stream_type"`
			FriendlyName       string `json:"friendly_name"`
			//SupportedFeatures  int    `json:"supported_features"`
		} `json:"attributes"`
		//LastChanged time.Time `json:"last_changed"`
		//LastUpdated time.Time `json:"last_updated"`
		//Context     struct {
		//	Id       string      `json:"id"`
		//	ParentId interface{} `json:"parent_id"`
		//	UserId   interface{} `json:"user_id"`
		//} `json:"context"`
	} `json:"result"`
}

type ResponseOffer struct {
	//Id      int    `json:"id"`
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Result  struct {
		Answer string `json:"answer"`
	} `json:"result"`
}

func SupervisorToken() string {
	return os.Getenv("SUPERVISOR_TOKEN")
}
