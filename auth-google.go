package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	goauth "google.golang.org/api/oauth2/v2"
)

type OAuthMessage struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	Message     string `json:"message"`
	Provider    string `json:"provider"`
	ProviderPic string `json:"provider_pic"`
	Token       string `json:"token"`
}

var (
	googleOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:4000/callback",
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{goauth.UserinfoEmailScope, goauth.UserinfoProfileScope},
		Endpoint:     google.Endpoint,
	}
	randomState = randToken()
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	url := googleOauthConfig.AuthCodeURL(randomState, oauth2.AccessTypeOffline)
	// fmt.Fprintf(w, url)
	urlstring := make(map[string]string)
	urlstring["url"] = url
	w.Header().Add("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(urlstring)
	// http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("state") != randomState {
		fmt.Println("state is not valid!")
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	token, err := googleOauthConfig.Exchange(oauth2.NoContext, r.FormValue("code"))
	if err != nil {
		fmt.Printf("Counldn't get token %s\n", err.Error())
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	res, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)

	if err != nil {
		fmt.Printf("Couldn't create a get request %s\n", err.Error())
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	defer res.Body.Close()
	content, err := ioutil.ReadAll(res.Body)

	if err != nil {
		fmt.Printf("Counldn't parse response %s\n", err.Error())
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	fmt.Fprint(w, string(content))
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {

}
