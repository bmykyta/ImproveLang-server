package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	r "github.com/dancannon/gorethink"
)

// User struct
type User struct {
	ID            string    `gorethink:"id,omitempty"`
	Name          string    `json:"name" gorethink:"name"`
	Username      string    `json:"username" gorethink:"username"`
	Email         string    `json:"email" gorethink:"email"`
	Password      string    `json:"password" gorethink:"password"`
	Token         string    `json:"token" gorethink:"token"`
	Provider      string    `json:"provider" gorethink:"provider"`
	Avatar        string    `json:"avatar" gorethink:"avatar"`
	AuthServiceID string    `json:"auth_service_id" gorethink:"auth_service_id" mapstructure:"auth_service_id"`
	Locale        string    `json:"locale" gorethink:"locale"`
	RoleID        string    `json:"role_id" gorethink:"role_id"`
	MilitaryTime  string    `json:"military_time" gorethink:"military_time"`
	Status        string    `json:"status" gorethink:"status"`
	CreatedAt     time.Time `json:"created_at" gorethink:"created_at"`
}

// UserRole Relation between user and role
type UserRole struct {
	UserID string `json:"user_id" gorethink:"user_id"`
	RoleID string `json:"role_id" gorethink:"role_id"`
}

// Role struct and db-table representation
type Role struct {
	ID          string `gorethink:"id,omitempty"`
	Name        string `json:"name" gorethink:"name"`
	Description string `json:"description" gorethink:"description"`
}

// Message struct
type Message struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

// Channel struct and db-table representation
type Channel struct {
	ID      string `json:"id" gorethink:"id,omitempty"`
	Name    string `json:"name" gorethink:"name"`
	Type    string `json:"type" gorethink:"type"`
	Purpose string `json:"purpose" gorethink:"purpose"`
}

func main() {
	session, err := r.Connect(r.ConnectOpts{
		Address:  "172.17.0.2:28015",
		Database: "improvelang",
	})
	if err != nil {
		log.Panic(err.Error())
	}
	router := NewRouter(session)

	router.Handle("channel add", addChannel)
	router.Handle("channel subscribe", subscribeChannel)
	router.Handle("channel unsubscribe", unsubscribeChannel)

	router.Handle("user edit", editUser)
	router.Handle("user subscribe", subscribeUser)
	router.Handle("user unsubscribe", unsubscribeUser)

	router.Handle("message add", addChannelMessage)
	router.Handle("message subscribe", subscribeChannelMessage)
	router.Handle("message unsubscribe", unsubscribeChannelMessage)

	router.Handle("google signup", googleSignUp)
	router.Handle("google login", googleLogin)
	router.Handle("check login", checkLogin)

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/callback", handleCallback)
	http.HandleFunc("/google/callback", handleGoogleCallback)

	http.HandleFunc("/setUser", handleSetUser)

	http.Handle("/chat", router)

	fmt.Println("Server start at port :4000")
	// http.ListenAndServe(":4000", nil)
	if err = http.ListenAndServeTLS(":4000", "./cert/server.crt", "./cert/server.key", nil); err != nil {
		log.Fatal(err.Error())
	}
}

func logFatalErr(err error) {
	if err != nil {
		log.Fatal(err)
		return
	}
}
