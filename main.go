package main

import (
	"context"
	"encoding/json"
	"fmt"

	"sync"
	"flag"
	"time"

	"github.com/acarl005/stripansi"

	"github.com/Tnze/go-mc/bot"
	"golang.org/x/sync/semaphore"
	"github.com/Tnze/go-mc/chat"
	"github.com/google/uuid"

	"go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type status struct {
	Description chat.Message
	Players     struct {
		Max    int
		Online int
		Sample []struct {
			ID   uuid.UUID
			Name string
		}
	}
	Version struct {
		Name     string
		Protocol int
	}
	Favicon Icon
	Delay   time.Duration
}
type Icon string

type ServerDB struct {
	ServerIP    string `bson:"serverIP"`
	Description string `bson:"description"`
	Version     string `bson:"version"`
	Players     struct {
		Max    int `bson:"max"`
		Online int `bson:"online"`
		List   []struct {
			ID    string `bson:"id"`
			Name  string `bson:"name"`
			LastSeen time.Time `bson:"lastSeen"`
		} `bson:"list"`
	} `bson:"players"`
	FoundAt time.Time `bson:"foundAt"`
	Favicon Icon   `bson:"favicon"`
	Alive   bool   `bson:"alive"`

}

type ServerDBbutMf struct {
	ServerIP    string `bson:"serverIP"`
	Description string `bson:"description"`
	Version     string `bson:"version"`
	Players     struct {
		Max    int `bson:"max"`
		Online int `bson:"online"`
		List   []struct {
			ID   uuid.UUID `bson:"id"`
			Name string    `bson:"name"`
			LastSeen time.Time `bson:"lastSeen"`
		} `bson:"list"`
	} `bson:"players"`
	FoundAt string `bson:"foundAt"`
	Favicon Icon   `bson:"favicon"`
	Alive	bool   `bson:"alive"`

}

var toupdate []mongo.WriteModel

func scanAndInsert(ip string, sem *semaphore.Weighted, wg *sync.WaitGroup, server *ServerDB) {
		defer wg.Done()
		defer sem.Release(1)
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("Recovered!")
			}
		}()
		resp, delay, err := bot.PingAndListTimeout(fmt.Sprint(ip), 5*time.Second)
		if err != nil {
			fmt.Printf("Ping and list server fail: %v\n", err)
			toupdate = append(toupdate, mongo.NewUpdateOneModel().SetFilter(bson.M{"serverIP": ip}).SetUpdate(bson.M{"$set": bson.M{"alive":false}}))

		}
		var s status
		err = json.Unmarshal(resp, &s)
		if err != nil {
			fmt.Print("Parse json response fail:\n", err)
		}
		s.Delay = delay
		if err == nil {
			if err != nil {
				fmt.Println(err)
			}
			serverbutmf := ServerDB{
				ServerIP:    ip,
				Description: stripansi.Strip(fmt.Sprint(s.Description)),
				Version:     s.Version.Name + " (" + fmt.Sprint(s.Version.Protocol) + ")",
				Players: struct {
					Max    int `bson:"max"`
					Online int `bson:"online"`
					List   []struct {
						ID   string `bson:"id"`
						Name string    `bson:"name"`
						LastSeen time.Time `bson:"lastSeen"`
					} `bson:"list"`
				}{
					Max:    s.Players.Max,
					Online: s.Players.Online,
					List: make([]struct {
						ID   string `bson:"id"`
						Name string    `bson:"name"`
						LastSeen time.Time `bson:"lastSeen"`
					}, 0),
				},
				FoundAt: time.Now(),
				Favicon: s.Favicon,
				Alive: true,

			}

			playerMap := make(map[string]time.Time)
			for _, player := range server.Players.List {
				playerMap[player.ID] = player.LastSeen
			}

			for _, playerSample := range s.Players.Sample {
				playerID := fmt.Sprint(playerSample.ID)
				lastSeen, exists := playerMap[playerID]
				if exists {

					lastSeen = time.Now()
					delete(playerMap, playerID) 
				} else {

					lastSeen = time.Now()
				}
				serverbutmf.Players.List = append(serverbutmf.Players.List, struct {
					ID       string    `bson:"id"`
					Name     string    `bson:"name"`
					LastSeen time.Time `bson:"lastSeen"`
				}{
					ID:       playerID,
					Name:     playerSample.Name,
					LastSeen: lastSeen,
				})
			}

			for _, player := range server.Players.List {
				if lastSeen, exists := playerMap[player.ID]; exists {

					serverbutmf.Players.List = append(serverbutmf.Players.List, struct {
						ID       string    `bson:"id"`
						Name     string    `bson:"name"`
						LastSeen time.Time `bson:"lastSeen"`
					}{
						ID:       player.ID,
						Name:     player.Name,
						LastSeen: lastSeen,
					})
					delete(playerMap, player.ID) 
				}
			}
			toupdate = append(toupdate, mongo.NewUpdateOneModel().SetFilter(bson.M{"serverIP": ip}).SetUpdate(bson.M{"$set": serverbutmf}))			

			fmt.Println("Server pinged!")
		}
	}

func main() {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("conn url"))
	if err != nil {
		fmt.Println(err)
	}
	semThr := flag.Int64("threads", 1500, "Max number of concurrent goroutines.")
	flag.Parse()
	sem := semaphore.NewWeighted(*semThr)

	coll := client.Database("db").Collection("coll")
	cursor, err := coll.Find(context.TODO(), bson.M{})
	if err != nil {
		fmt.Println(err)
	}
	var s []ServerDB
	if err = cursor.All(context.TODO(), &s); err != nil {
		fmt.Println(err)
	}
	var wg sync.WaitGroup
	for _, serverres := range s {
		cursor.Decode(&serverres)
		wg.Add(1)
		sem.Acquire(context.Background(), 1)
		go scanAndInsert(serverres.ServerIP, sem, &wg, &serverres)
	}
	wg.Wait()

}
