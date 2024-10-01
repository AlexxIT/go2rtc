package tuya

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/tidwall/gjson"
)

// Rest 向开放平台发送HTTP请求，返回开放平台回复的payload给上层
func Rest(method string, url string, body io.Reader) (res []byte, err error) {
	client := &http.Client{
		Timeout: time.Second * 5,
	}

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Printf("create http request fail: %s", err.Error())

		return
	}

	ts := time.Now().UnixNano() / 1000000
	sign := calBusinessSign(ts)

	request.Header.Set("Accept", "*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Access-Control-Allow-Origin", "*")
	request.Header.Set("Access-Control-Allow-Methods", "*")
	request.Header.Set("Access-Control-Allow-Headers", "*")
	request.Header.Set("mode", "no-cors")
	request.Header.Set("client_id", App.ClientID)
	request.Header.Set("access_token", App.AccessToken)
	request.Header.Set("sign", sign)
	request.Header.Set("t", strconv.FormatInt(ts, 10))

	response, err := client.Do(request)
	if err != nil {
		log.Printf("http request fail: %s", err.Error())

		return
	}
	defer response.Body.Close()

	res, err = io.ReadAll(response.Body)
	if err != nil {
		log.Printf("read http response fail", err.Error())

		return
	}

	return
}

func RestToken(method string, url string, body io.Reader) (res []byte, err error) {
	client := &http.Client{
		Timeout: time.Second * 5,
	}

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Printf("create http request fail: %s", err.Error())

		return
	}

	ts := time.Now().UnixNano() / 1000000
	sign := calTokenSign(ts)

	request.Header.Set("Accept", "*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Access-Control-Allow-Origin", "*")
	request.Header.Set("Access-Control-Allow-Methods", "*")
	request.Header.Set("Access-Control-Allow-Headers", "*")
	request.Header.Set("mode", "no-cors")
	request.Header.Set("client_id", App.ClientID)
	request.Header.Set("access_token", App.AccessToken)
	request.Header.Set("sign", sign)
	request.Header.Set("t", strconv.FormatInt(ts, 10))

	response, err := client.Do(request)
	if err != nil {
		log.Printf("http request fail: %s", err.Error())

		return
	}
	defer response.Body.Close()

	res, err = io.ReadAll(response.Body)
	if err != nil {
		log.Printf("read http response fail", err.Error())

		return
	}

	return
}

func InitToken() (err error) {
	var url = fmt.Sprintf("%s/v1.0/token?grant_type=1", App.OpenAPIURL)

	body, err := Rest("GET", url, nil)
	if err != nil {
		log.Printf("GET token fail: %s, body: %s", err.Error(), string((body)))

		return
	}

	err = syncToConfig(body)
	if err != nil {
		log.Printf("sync OpenAPI ressponse to config fail: %s", err.Error())

		return
	}

	// 启动token维护更新协程
	go maintainToken()

	return
}

func refreshToken() (err error) {
	url := fmt.Sprintf("%s/v1.0/token/%s", App.OpenAPIURL, App.RefreshToken)

	body, err := RestToken("GET", url, nil)
	if err != nil {
		log.Printf("GET token fail: %s, body: %s", err.Error(), string((body)))

		return
	}

	err = syncToConfig(body)
	if err != nil {
		log.Printf("sync OpenAPI ressponse to config fail: %s", err.Error())

		return
	}

	return
}

func syncToConfig(body []byte) error {
	uIdValue := gjson.GetBytes(body, "result.uid")
	if !uIdValue.Exists() {
		log.Printf("uid not exits in body: %s", string(body))

		return errors.New("uid not exist")
	}

	accessTokenValue := gjson.GetBytes(body, "result.access_token")
	if !accessTokenValue.Exists() {
		log.Printf("access_token not exits in body: %s", string(body))

		return errors.New("access_token not exist")
	}

	refreshTokenValue := gjson.GetBytes(body, "result.refresh_token")
	if !refreshTokenValue.Exists() {
		log.Printf("refresh_token not exist")

		return errors.New("refresh_token not exist")
	}

	expireTimeValue := gjson.GetBytes(body, "result.expire_time")
	if !expireTimeValue.Exists() {
		log.Printf("expire_time not exist")

		return errors.New("expire_time not exist")
	}

	App.AccessToken = accessTokenValue.String()
	App.RefreshToken = refreshTokenValue.String()
	App.ExpireTime = expireTimeValue.Int()

	return nil
}

func maintainToken() {
	interval := App.ExpireTime - 300

	for {
		timer := time.NewTimer(time.Duration(interval) * time.Second)

		select {
		case <-timer.C:
			if err := refreshToken(); err != nil {
				log.Printf("refresh token fail: %s", err.Error())

				interval = 60
			} else {
				interval = App.ExpireTime - 300
			}
		}
	}
}
