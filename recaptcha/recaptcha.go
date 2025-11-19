package recaptcha

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

type response struct {
	Success     bool      `json:"success"`
	Score       float64   `json:"score"`
	Action      string    `json:"action"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
}

const recaptchaApi = "https://www.google.com/recaptcha/api/siteverify"

var (
	recaptchaEnable    bool
	recaptchaSecretKey string
)

// check uses the client ip address, the challenge code from the reCaptcha form,
// and the client's response input to that challenge to determine whether
// the client answered the reCaptcha input question correctly.
// It returns a boolean value indicating whether the client answered correctly.
func check(ip, response string) (r response, err error) {
	resp, err := http.PostForm(recaptchaApi,
		url.Values{"secret": {recaptchaSecretKey}, "remoteip": {ip}, "response": {response}})
	if err != nil {
		log.Printf("Post error: %s\n", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read error: could not read body: %s\n", err)
		return
	}
	err = json.Unmarshal(body, &r)
	if err != nil {
		log.Printf("Read error: got invalid JSON: %s\n", err)
		return
	}
	return
}

// confirm calls check, which the client ip address, the challenge code from the reCaptcha form,
// and the client's response input to that challenge to determine whether
// the client answered the reCaptcha input question correctly.
// It returns a boolean value indicating whether the client answered correctly.
func confirm(response string) (result bool, err error) {
	resp, err := check("", response)
	result = resp.Success
	return
}

// New allows the webserver or code evaluating the reCaptcha form input to set the
// reCaptcha private key (string) value, which will be different for every domain.
func New(enable bool, key string) {
	recaptchaEnable = enable
	recaptchaSecretKey = key
}

func Validate(token string) error {
	if recaptchaEnable {
		if recaptchaSecretKey == "" {
			return errors.New("invalid recaptcha configuration")
		}

		if len(token) == 0 {
			return errors.New("invalid recaptcha token")
		}

		ok, err := confirm(token)
		if !ok {
			if err != nil {
				return err
			}
			return errors.New("recaptcha failed, please try again")
		}
	}
	return nil
}
