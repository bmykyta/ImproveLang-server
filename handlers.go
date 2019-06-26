package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	r "github.com/dancannon/gorethink"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/olivere/elastic.v6"
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

type ChannelSearch struct {
	SearchRequest string `json:"channel"`
}

type ElasticResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Type    string `json:"type"`
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

func changeUsername(client *Client, data interface{}) {
	username := data.(map[string]interface{})
	client.user.Username = username["username"].(string)
	client.userName = username["username"].(string)

	go func() {
		err := r.Table("user").
			Update(client.user).
			Exec(client.session)

		if err != nil {
			client.send <- Message{"error", err.Error()}
		}
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

func deleteChannelMessage(client *Client, data interface{}) {
	var channelMessage ChannelMessage
	err := mapstructure.Decode(data, &channelMessage)
	if err != nil {
		client.send <- Message{"error", err.Error()}
	}
	go func() {
		err := r.Table("message").
			Get(channelMessage.Id).
			Delete().
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
	channel.Type = "public"
	go func() {
		err = r.Table("channel").
			Insert(channel).
			Exec(client.session)

		if err != nil {
			client.send <- Message{"error", err.Error()}
		}
		put1, err := client.elasticClient.Index().
			Index("channels").
			Type("channel").
			BodyJson(channel).
			Do(context.Background())
		fmt.Printf("elastica index %#v\n", put1)
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
	if resultExist == true {
		return
	}
	usr := strings.Split(strings.ToLower(user.Name), " ")
	user.Status = "Online"
	user.Username = usr[0]
	if usr[1] != "" {
		user.Username += "_" + usr[1]
	}
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

func searchChannels(client *Client, data interface{}) {
	searchQuery := data.(map[string]interface{})

	// search, err := client.elasticClient.
	// 	Get().
	// 	Index("channels").Type("channel").Id("123").
	// 	Do(context.Background())
	// var channelSearch ChannelSearch

	// indexParams := `{
	// 	"settings":{
	// 		"number_of_shards":1,
	// 		"number_of_replicas":0
	// 	},
	// 	"mappings":{
	// 		"channel":{
	// 			"properties": {
	// 				"name":{
	// 					"type":"text"
	// 				},
	// 				"purpose":{
	// 					"type":"text"
	// 				},
	// 				"channel_type":{
	// 					"type":"text"
	// 				}
	// 			}
	// 		}
	// 	}
	// }`
	// createRes, err := client.elasticClient.CreateIndex("channels").BodyString(indexParams).Do(context.Background())
	// if err != nil {
	// 	panic(err)
	// }

	// err := json.Unmarshal(data.([]byte), &channelSearch)
	searchQuery["channel"] = strings.ToLower(strings.TrimSpace(searchQuery["channel"].(string)))
	termQuery := elastic.NewTermQuery("purpose", searchQuery["channel"])

	searchChannel, err := client.elasticClient.
		Search().
		Index("channels").
		Query(termQuery).
		// Sort("name", true).
		// From(0).Size(10).
		Do(context.Background())

	if err != nil {
		switch {
		case elastic.IsNotFound(err):
			panic(fmt.Sprintf("Document not found: %v", err))
		case elastic.IsTimeout(err):
			panic(fmt.Sprintf("Timeout retrieving document: %v", err))
		case elastic.IsConnErr(err):
			panic(fmt.Sprintf("Connection problem: %v", err))
		default:
			panic(err)
		}
	}

	elasticResultSlice := []ElasticResult{}
	elResult := ElasticResult{}

	if searchChannel.TotalHits() > 0 {

		for _, hit := range searchChannel.Hits.Hits {
			// Deserialize hit.Source into a channel data (could also be just a map[string]interface{}).
			err := json.Unmarshal(*hit.Source, &elResult)
			if err != nil {
				fmt.Printf("There might be an error %v", err.Error())
			}
			elResult.ID = hit.Id
			elasticResultSlice = append(elasticResultSlice, elResult)
		}
		client.send <- Message{"more channels", elasticResultSlice}

	} else {
		fmt.Print("Found no channels\n")
		client.send <- Message{"error", "No channels found"}
	}
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
	response["id"] = user.ID

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

func contactForm(client *Client, data interface{}) {
	formValues := data.(map[string]interface{})
	fmt.Printf("contact form %#v\n", formValues)
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
