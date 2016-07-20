package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type Item struct {
	Id    string `bson:"_id"`
	Path  string `bson:"path"`
	Queue uint32 `bson:"queue"`
	Plays uint32 `bson:"plays"`
}

var paths []string

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewId(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func visit(path string, f os.FileInfo, err error) error {
	if f.Mode().IsRegular() {
		if filepath.Ext(path) == ".mp3" {
			paths = append(paths, path)
		}
	}
	return nil
}

func loadPaths(userPath string) {

	// userPath = "/Users/jsgoyette/Data/Downloads"

	if userPath == "" {
		fmt.Println("missing file path")
		os.Exit(2)
	}

	// check that userPath exists
	if _, err := os.Stat(userPath); err != nil {
		fmt.Println(userPath, err)
		os.Exit(2)
	}

	// load the files into `paths`
	err := filepath.Walk(userPath, visit)
	if err != nil {
		panic(err)
	}
}

func main() {

	flag.Parse()
	userPath := flag.Arg(0)

	loadPaths(userPath)

	// start mongo connection
	session, err := mgo.Dial("127.0.0.1")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	c := session.DB("playlist").C("items")

	// grab the current highest `queue`
	var highestQueuedItem Item
	err = c.Find(bson.M{}).Sort("-queue").One(&highestQueuedItem)
	if err != nil {
		fmt.Println("could not find highest queue", err)
	}

	queue := highestQueuedItem.Queue

	// for each path found, check if it exists
	// only insert if it does not already exist
	for _, path := range paths {

		// skip if already loaded
		if count, _ := c.Find(bson.M{"path": path}).Count(); count > 0 {
			fmt.Printf("%v SKIPPING\n", path)
			continue
		}

		queue++

		item := Item{
			Id:    NewId(18),
			Path:  path,
			Queue: queue,
			Plays: 0,
		}

		fmt.Printf("%v\n", path)

		// insert item
		if err = c.Insert(item); err != nil {
			fmt.Println("could not insert", err)
		}
	}
}
