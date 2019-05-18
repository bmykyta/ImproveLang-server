package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/mitchellh/mapstructure"
)

const (
	ChannelStop = iota
	UserStop
	MessageStop
)

type ChannelMessage struct {
	Id          string    `gorethink:"id,omitempty"`
	ChannelId   string    `gorethink:"channel_id"`
	Body        string    `gorethink:"body"`
	Author      string    `gorethink:"author"`
	Username    string    `gorethink:"username"`
	UserID      string    `gorethink:"user_id"`
	Attachments string    `gorethink:"attachments"`
	CreatedAt   time.Time `gorethink:"created_at"`
}

func editUser(client *Client, data interface{}) {
	var user User
	err := mapstructure.Decode(data, &user)
	usr := data.(map[string]interface{})
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}

	client.userName = usr["name"].(string)
	go func() {
		_, err := r.Table("user").
			Get(client.id).
			Update(usr).
			RunWrite(client.session)
		if err != nil {
			client.send <- Message{"error", err.Error()}
		}
	}()
}

func subscribeUser(client *Client, data interface{}) {
	go func() {
		stop := client.NewStopChannel(UserStop)
		cursor, err := r.Table("user").
			Changes(r.ChangesOpts{IncludeInitial: true}).
			Run(client.session)

		if err != nil {
			client.send <- Message{"error", err.Error()}
			return
		}
		changeFeedHelper(cursor, "user", client.send, stop)
	}()
}

func unsubscribeUser(client *Client, data interface{}) {
	client.StopForKey(UserStop)
}

func addChannelMessage(client *Client, data interface{}) {
	var channelMessage ChannelMessage
	err := mapstructure.Decode(data, &channelMessage)
	if err != nil {
		client.send <- Message{"error", err.Error()}
	}
	go func() {
		channelMessage.CreatedAt = time.Now()
		channelMessage.Author = client.userName
		channelMessage.UserID = client.user.ID
		channelMessage.Username = client.user.Username
		err := r.Table("message").
			Insert(channelMessage).
			Exec(client.session)

		if err != nil {
			client.send <- Message{"error", err.Error()}
		}
	}()
}

func subscribeChannelMessage(client *Client, data interface{}) {
	go func() {
		eventData := data.(map[string]interface{})
		val, ok := eventData["channelId"]
		if !ok {
			return
		}
		channelId, ok := val.(string)
		if !ok {
			return
		}
		stop := client.NewStopChannel(MessageStop)
		cursor, err := r.Table("message").
			OrderBy(r.OrderByOpts{Index: "created_at"}).
			Limit(100).
			Filter(r.Row.Field("channel_id").Eq(channelId)).
			Changes(r.ChangesOpts{IncludeInitial: true}).
			Run(client.session)

		if err != nil {
			client.send <- Message{"error", err.Error()}
			return
		}
		changeFeedHelper(cursor, "message", client.send, stop)
	}()
}

func unsubscribeChannelMessage(client *Client, data interface{}) {
	client.StopForKey(MessageStop)
}

func addChannel(client *Client, data interface{}) {
	var channel Channel
	err := mapstructure.Decode(data, &channel)
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}
	go func() {
		err = r.Table("channel").
			Insert(channel).
			Exec(client.session)
		if err != nil {
			client.send <- Message{"error", err.Error()}
		}
	}()
}

func subscribeChannel(client *Client, data interface{}) {
	go func() {
		stop := client.NewStopChannel(ChannelStop)
		cursor, err := r.Table("channel").
			Changes(r.ChangesOpts{IncludeInitial: true}).
			Run(client.session)
		if err != nil {
			client.send <- Message{"error", err.Error()}
			return
		}
		changeFeedHelper(cursor, "channel", client.send, stop)
	}()
}

func googleSignUp(client *Client, data interface{}) {
	var user User
	err := mapstructure.Decode(data, &user)
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}
	// fmt.Printf("sign USER DATA %#v\n", data)
	// fmt.Printf("signed USER  %#v\n", user)
	// check if user exists
	cursor, err := r.Table("user").
		Filter(map[string]interface{}{"auth_service_id": user.AuthServiceID}).
		Run(client.session)
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}
	defer cursor.Close()
	var change r.ChangeResponse
	resultExist := false
	for cursor.Next(&change) {
		err = mapstructure.Decode(change.NewValue, &user)
		if err != nil {
			log.Fatal(err.Error())
		}
		fmt.Println("User exists!!")
		client.user = user
		client.id = user.ID
		client.userName = user.Name
		resultExist = true
	}
	if resultExist == true {
		return
	}
	usr := strings.Split(strings.ToLower(user.Name), " ")
	user.Status = "Online"
	user.Username = usr[0] + "_" + usr[1]
	user.CreatedAt = time.Now()

	res, err := r.Table("user").Insert(user).RunWrite(client.session)
	if err != nil {
		client.send <- Message{"error", err.Error()}
	}
	var id string
	if len(res.GeneratedKeys) > 0 {
		id = res.GeneratedKeys[0]
	}

	client.userName = user.Name
	client.id = id
	client.user = user

	// client.send <- Message{"sign up", "Registration completed!"}
}

func googleLogin(client *Client, data interface{}) {
	var user User
	err := mapstructure.Decode(data, &user)
	if err != nil {
		fmt.Println(err.Error())
	}
	cursor, err := r.Table("user").
		Filter(map[string]interface{}{"auth_service_id": user.AuthServiceID}).
		Run(client.session)
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}
	defer cursor.Close()
	var change r.ChangeResponse
	resultExist := false
	for cursor.Next(&change) {
		err = mapstructure.Decode(change.NewValue, &user)
		if err != nil {
			log.Fatal(err.Error())
		}
		client.user = user
		client.id = user.ID
		client.userName = user.Name
		resultExist = true
	}
	if resultExist == false {
		googleSignUp(client, data)
	}

	// client.send <- Message{"check login", "User logged in!"}
}

func checkLogin(client *Client, data interface{}) {
	var user User
	err := mapstructure.Decode(data, &user)
	if err != nil {
		log.Fatal(err.Error())
	}
	if user.AuthServiceID == "" {
		client.send <- Message{"check login", "Not logged in"}
		return
	}
	cursor, err := r.Table("user").
		Filter(map[string]interface{}{"auth_service_id": user.AuthServiceID}).
		Changes(r.ChangesOpts{IncludeInitial: true}).
		Run(client.session)
	if err != nil {
		client.send <- Message{"error", err.Error()}
		return
	}
	defer cursor.Close()
	var change r.ChangeResponse
	for cursor.Next(&change) {
		err = mapstructure.Decode(change.NewValue, &user)
		if err != nil {
			log.Fatal(err.Error())
		}
		client.user = user
		client.id = user.ID
		client.userName = user.Name
		cursor.Close()
	}
	response := make(map[string]interface{})
	response["name"] = user.Name
	response["avatar"] = user.Avatar
	response["username"] = user.Username
	response["status"] = user.Status
	response["created_at"] = user.CreatedAt
	client.send <- Message{"check login", response}
}

func unsubscribeChannel(client *Client, data interface{}) {
	client.StopForKey(ChannelStop)
}

func changeFeedHelper(cursor *r.Cursor, changeEventName string, send chan<- Message, stop <-chan bool) {
	change := make(chan r.ChangeResponse)
	cursor.Listen(change)
	for {
		eventName := ""
		var data interface{}
		select {
		case <-stop:
			cursor.Close()
			return
		case val := <-change:
			if val.NewValue != nil && val.OldValue == nil {
				eventName = changeEventName + " add"
				data = val.NewValue
			} else if val.NewValue == nil && val.OldValue != nil {
				eventName = changeEventName + " remove"
				data = val.OldValue
			} else if val.NewValue != nil && val.OldValue != nil {
				eventName = changeEventName + " edit"
				data = val.NewValue
			}
			send <- Message{eventName, data}
		}
	}
}

func Check(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		if (*r).Method == "OPTIONS" {
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func handleSetUser(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Post data")
}
